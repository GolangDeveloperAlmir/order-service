package main

import (
	"context"
	"github.com/GolangDeveloperAlmir/order-service/internal/app"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
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
			return
		}
	}()

	if err := app.Run(ctx, cfg, logger); err != nil {
		return
	}

	time.Sleep(100 * time.Millisecond)
}
