package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/config"
	"github.com/techize/selloovy/internal/database"
	"github.com/techize/selloovy/internal/server"
	"github.com/techize/selloovy/internal/shop"
	"github.com/techize/selloovy/internal/shophttp"
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
	var authentication http.Handler
	var adminAPI http.Handler
	if cfg.DatabaseURL != "" {
		pool, err := database.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		ready = func(ctx context.Context) error { return database.Ready(ctx, pool) }
		if cfg.AuthKeyFile != "" {
			key, e := auth.LoadKeyFile(cfg.AuthKeyFile)
			if e != nil {
				return e
			}
			vault, e := auth.NewMFAVault(key)
			clear(key)
			if e != nil {
				return e
			}
			store, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
			if e != nil {
				return errors.New("could not initialize authentication")
			}
			protected, e := authhttp.New(store, cfg.PublicOrigin)
			if e != nil {
				return e
			}
			authentication = protected
			adminAPI = protected.Protect(shophttp.New(shop.NewStore(pool)))
		}
	}
	handler, err := web.NewHandler(ready, os.DirFS(cfg.AdminDir), authentication, adminAPI)
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
