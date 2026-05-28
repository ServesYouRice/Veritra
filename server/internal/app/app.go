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
	return privacyHeaders(rateLimit(mux))
}

func (a *App) Serve(ctx context.Context) error {
	server := &http.Server{
		Addr:              a.Config.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	a.Log.Info("server_starting", "addr", a.Config.Addr)
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) Close() error {
	return a.Store.Close()
}

func privacyHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func rateLimit(next http.Handler) http.Handler {
	var salt [16]byte
	_, _ = rand.Read(salt[:])
	type bucket struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	buckets := map[string]bucket{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := remoteHash(salt[:], r.RemoteAddr)
		now := time.Now()
		mu.Lock()
		current := buckets[key]
		if current.reset.Before(now) {
			current = bucket{reset: now.Add(time.Minute)}
		}
		current.count++
		buckets[key] = current
		for k, b := range buckets {
			if b.reset.Before(now) {
				delete(buckets, k)
			}
		}
		allowed := current.count <= 240
		mu.Unlock()
		if !allowed {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteHash(salt []byte, remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	sum := sha256.Sum256(append(salt, []byte(host)...))
	return hex.EncodeToString(sum[:])
}
