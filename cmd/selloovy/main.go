package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/techize/selloovy/internal/config"
	"github.com/techize/selloovy/internal/database"
	"github.com/techize/selloovy/internal/server"
	"github.com/techize/selloovy/internal/web"
)

func main() {
	if err := run(); err != nil {
		// Do not dump configuration or wrapped network errors to logs.
		slog.Error("application stopped", "reason", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var ready web.ReadinessCheck
	if cfg.DatabaseURL != "" {
		pool, err := database.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		ready = func(ctx context.Context) error { return database.Ready(ctx, pool) }
	}
	handler, err := web.NewHandler(ready)
	if err != nil {
		return errors.New("could not initialize storefront")
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("could not open HTTP listener")
	}
	slog.Info("foundation started", "database_configured", ready != nil)
	if err := server.Run(ctx, listener, handler); err != nil {
		return errors.New("HTTP server failed to stop cleanly")
	}
	slog.Info("application stopped cleanly")
	return nil
}
