package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := log.New(cfg.AppEnv)
	defer func() {
		if err := logger.Sync(); err != nil {
			logger.Error("failed to sync logger", "error", err)
		}
	}()

	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Panic("failed to run application", "error", err)
	}

	time.Sleep(100 * time.Millisecond)
}
