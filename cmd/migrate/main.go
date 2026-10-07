package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/techize/selloovy/internal/config"
	"github.com/techize/selloovy/internal/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migration stopped", "reason", err.Error())
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 1 {
		return errors.New("migration command accepts no arguments and only moves forward")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("SELLOOVY_DATABASE_URL is required for migration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	slog.Info("database schema is up to date")
	return nil
}
