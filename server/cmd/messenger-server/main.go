package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"private-messenger/server/internal/app"
	"private-messenger/server/internal/config"
	"private-messenger/server/internal/storage"
	"private-messenger/server/migrations"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stdout)
		return nil
	}
	command := args[0]
	cfg := config.Load()
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dbPath string
	var storagePath string
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory")
	fs.StringVar(&dbPath, "db", "", "SQLite database path")
	fs.StringVar(&storagePath, "storage", "", "encrypted blob storage path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if dbPath != "" {
		cfg.DatabasePath = dbPath
	} else {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "private-messenger.db")
	}
	if storagePath != "" {
		cfg.StoragePath = storagePath
	} else {
		cfg.StoragePath = filepath.Join(cfg.DataDir, "blobs")
	}

	ctx := context.Background()
	switch command {
	case "serve":
		return serve(ctx, cfg)
	case "init":
		return initInstance(ctx, cfg, stdout)
	case "migrate":
		return migrate(ctx, cfg, stdout)
	case "doctor":
		return doctor(ctx, cfg, stdout)
	case "backup":
		return backup(cfg, fs.Args(), stdout)
	case "restore":
		return restore(cfg, fs.Args(), stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func serve(ctx context.Context, cfg config.Config) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()
	return application.Serve(ctx)
}

func initInstance(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.StoragePath, 0o700); err != nil {
		return err
	}
	if err := migrate(ctx, cfg, io.Discard); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "initialized data directory: %s\nsetup URL: http://localhost%s/setup\n", cfg.DataDir, cfg.Addr)
	return nil
}

func migrate(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx, migrations.FS); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "migrations applied")
	return nil
}

func doctor(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Check(ctx); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "storage: ok")
	fmt.Fprintln(stdout, "telemetry: disabled")
	fmt.Fprintln(stdout, "message plaintext persistence: forbidden by schema/API")
	return nil
}

func backup(cfg config.Config, args []string, stdout io.Writer) error {
	out := filepath.Join(cfg.DataDir, "backups", "private-messenger-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if len(args) > 0 {
		out = args[0]
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	if err := copyFile(cfg.DatabasePath, out, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database backup written: %s\n", out)
	fmt.Fprintln(stdout, "note: encrypted blob directory backup is still operator-managed in this MVP")
	return nil
}

func restore(cfg config.Config, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("restore requires path to a database backup")
	}
	if err := copyFile(args[0], cfg.DatabasePath, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "database restored to: %s\n", cfg.DatabasePath)
	fmt.Fprintln(stdout, "note: stop the server before restore and restore encrypted blobs separately")
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Private Messenger server")
	fmt.Fprintln(w, "commands: serve, init, migrate, backup, restore, doctor")
}
