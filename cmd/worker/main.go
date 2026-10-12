package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/config"
	"github.com/techize/selloovy/internal/database"
	"github.com/techize/selloovy/internal/paymentvault"
	"github.com/techize/selloovy/internal/square"
)

func main() {
	if e := run(); e != nil {
		slog.Error("worker stopped", "reason", e.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.Load(os.LookupEnv)
	if e != nil {
		return e
	}
	if cfg.DatabaseURL == "" {
		return errors.New("worker requires a configured database")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, e := database.Open(ctx, cfg.DatabaseURL)
	if e != nil {
		return e
	}
	defer pool.Close()
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	e = database.Ready(check, pool)
	cancel()
	if e != nil {
		return errors.New("worker database schema unavailable")
	}
	store := catalog.NewStore(pool)
	var payments *catalog.Payments
	if cfg.SquareTokenFile != "" {
		root, e := auth.LoadKeyFile(cfg.AuthKeyFile)
		if e != nil {
			return e
		}
		vault, e := paymentvault.New(root)
		if e != nil {
			clear(root)
			return e
		}
		client, e := square.NewSandbox(cfg.SquareTokenFile, cfg.SquareLocationID)
		if e != nil {
			clear(root)
			return e
		}
		payments, e = catalog.NewPayments(store, vault, client, root)
		clear(root)
		if e != nil {
			return e
		}
	}
	slog.Info("worker started", "sandbox_reconciliation", payments != nil)
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		maintenance, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, sourceErr := store.EraseExpiredPaymentSources(maintenance)
		_, budgetErr := store.PurgePaymentBudgets(maintenance)
		cancel()
		if ctx.Err() != nil {
			break
		}
		if sourceErr != nil || budgetErr != nil {
			slog.Warn("payment maintenance will retry")
		}
		if payments != nil {
			if _, e = payments.ReconcileDue(ctx); e != nil && ctx.Err() == nil {
				slog.Warn("payment reconciliation will retry")
			}
		}
		select {
		case <-ctx.Done():
			slog.Info("worker stopped cleanly")
			return nil
		case <-tick.C:
		}
	}
	slog.Info("worker stopped cleanly")
	return nil
}
