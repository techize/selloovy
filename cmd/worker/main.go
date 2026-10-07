package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("worker scaffold started; no jobs are processed")
	<-ctx.Done()
	slog.Info("worker stopped cleanly")
}
