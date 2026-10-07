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
	handler, err := web.NewHandler(nil)
	if err != nil {
		return errors.New("could not initialize storefront")
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("could not open HTTP listener")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("foundation started; database readiness is disabled")
	if err := server.Run(ctx, listener, handler); err != nil {
		return errors.New("HTTP server failed to stop cleanly")
	}
	slog.Info("application stopped cleanly")
	return nil
}
