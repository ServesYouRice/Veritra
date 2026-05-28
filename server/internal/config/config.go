package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr         string
	DataDir      string
	DatabasePath string
	StoragePath  string
	InstanceName string
}

func Load() Config {
	cfg := Config{
		Addr:         getenv("PRIVATE_MESSENGER_ADDR", ":8080"),
		DataDir:      getenv("PRIVATE_MESSENGER_DATA_DIR", "./data"),
		InstanceName: getenv("PRIVATE_MESSENGER_INSTANCE_NAME", "Private Messenger"),
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
