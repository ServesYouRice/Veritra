package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr           string
	DataDir        string
	DatabasePath   string
	StoragePath    string
	InstanceName   string
	TrustedProxies []*net.IPNet
}

func Load() Config {
	cfg := Config{
		Addr:           getenv("PRIVATE_MESSENGER_ADDR", ":8080"),
		DataDir:        getenv("PRIVATE_MESSENGER_DATA_DIR", "./data"),
		InstanceName:   getenv("PRIVATE_MESSENGER_INSTANCE_NAME", "Private Messenger"),
		TrustedProxies: parseCIDRs(getenv("PRIVATE_MESSENGER_TRUSTED_PROXIES", "")),
	}
	cfg.DatabasePath = getenv("PRIVATE_MESSENGER_DB_PATH", filepath.Join(cfg.DataDir, "private-messenger.db"))
	cfg.StoragePath = getenv("PRIVATE_MESSENGER_STORAGE_PATH", filepath.Join(cfg.DataDir, "blobs"))
	return cfg
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseCIDRs(raw string) []*net.IPNet {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if strings.Contains(part, ":") {
				part += "/128"
			} else {
				part += "/32"
			}
		}
		_, cidr, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		result = append(result, cidr)
	}
	return result
}
