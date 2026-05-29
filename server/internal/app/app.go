package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"private-messenger/server/internal/config"
	"private-messenger/server/internal/httpapi"
	"private-messenger/server/internal/realtime"
	"private-messenger/server/internal/storage"
	"private-messenger/server/internal/uploads"
	"private-messenger/server/migrations"
)

type App struct {
	Config config.Config
	Store  *storage.Store
	Hub    *realtime.Hub
	Blobs  *uploads.LocalStore
	Log    *slog.Logger
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	}
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx, migrations.FS); err != nil {
		_ = store.Close()
		return nil, err
	}
	blobs, err := uploads.NewLocalStore(cfg.StoragePath)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &App{Config: cfg, Store: store, Hub: realtime.NewHub(), Blobs: blobs, Log: logger}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	api := &httpapi.API{Store: a.Store, Hub: a.Hub, Blobs: a.Blobs, Log: a.Log}
	api.Register(mux)
	limiter := newRateLimiter(a.Config.TrustedProxies, 240, 10)
	return securityHeaders(limiter.middleware(mux))
}

func (a *App) Serve(ctx context.Context) error {
	server := &http.Server{
		Addr:              a.Config.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go a.runRetentionSweeper(ctx)
	a.Log.Info("server_starting", "addr", a.Config.Addr)
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// runRetentionSweeper periodically prunes sync_events and audit_events older
// than the retention window. The window is 30 days by default and can be
// overridden via PRIVATE_MESSENGER_SYNC_EVENT_RETENTION_DAYS.
func (a *App) runRetentionSweeper(ctx context.Context) {
	retention := 30 * 24 * time.Hour
	if raw := os.Getenv("PRIVATE_MESSENGER_SYNC_EVENT_RETENTION_DAYS"); raw != "" {
		if days, err := strconv.Atoi(raw); err == nil && days > 0 {
			retention = time.Duration(days) * 24 * time.Hour
		}
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	sweep := func() {
		cutoff := time.Now().UTC().Add(-retention)
		if removed, err := a.Store.PruneSyncEvents(ctx, cutoff); err != nil {
			a.Log.Warn("sync_event_prune_failed", "err", err)
		} else if removed > 0 {
			a.Log.Info("sync_events_pruned", "removed", removed)
		}
		if removed, err := a.Store.PruneAuditEvents(ctx, cutoff); err != nil {
			a.Log.Warn("audit_event_prune_failed", "err", err)
		} else if removed > 0 {
			a.Log.Info("audit_events_pruned", "removed", removed)
		}
	}
	sweep()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func (a *App) Close() error {
	return a.Store.Close()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/setup") {
			h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	salt           [16]byte
	trustedProxies []*net.IPNet
	generalLimit   int
	authLimit      int

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	general int
	auth    int
	reset   time.Time
}

const maxRateLimitEntries = 65536

func newRateLimiter(trustedProxies []*net.IPNet, general, auth int) *rateLimiter {
	rl := &rateLimiter{
		trustedProxies: trustedProxies,
		generalLimit:   general,
		authLimit:      auth,
		buckets:        map[string]*bucket{},
	}
	_, _ = rand.Read(rl.salt[:])
	go rl.cleanupLoop()
	return rl
}

func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if b.reset.Before(now) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := rl.clientIP(r)
		key := remoteHash(rl.salt[:], clientIP)
		now := time.Now()
		auth := isAuthEndpoint(r.URL.Path)

		rl.mu.Lock()
		b, ok := rl.buckets[key]
		if !ok || b.reset.Before(now) {
			if len(rl.buckets) >= maxRateLimitEntries && !ok {
				rl.mu.Unlock()
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			b = &bucket{reset: now.Add(time.Minute)}
			rl.buckets[key] = b
		}
		b.general++
		if auth {
			b.auth++
		}
		overGeneral := b.general > rl.generalLimit
		overAuth := auth && b.auth > rl.authLimit
		rl.mu.Unlock()

		if overGeneral || overAuth {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAuthEndpoint(path string) bool {
	switch path {
	case "/api/v1/setup/owner",
		"/api/v1/auth/login",
		"/api/v1/register",
		"/api/v1/device-links/claim":
		return true
	}
	return strings.HasPrefix(path, "/api/v1/device-links/") && strings.HasSuffix(path, "/claim-status")
}

func (rl *rateLimiter) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(rl.trustedProxies) == 0 {
		return host
	}
	directIP := net.ParseIP(host)
	if directIP == nil || !ipInAny(directIP, rl.trustedProxies) {
		return host
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		if !ipInAny(ip, rl.trustedProxies) {
			return candidate
		}
	}
	return strings.TrimSpace(parts[0])
}

func ipInAny(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteHash(salt []byte, host string) string {
	sum := sha256.Sum256(append(salt, []byte(host)...))
	return hex.EncodeToString(sum[:])
}
