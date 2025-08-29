package app

import (
	"context"
	"fmt"
	"github.com/GolangDeveloperAlmir/order-service/internal/config"
	"github.com/GolangDeveloperAlmir/order-service/internal/order/repository/postgres"
	"github.com/GolangDeveloperAlmir/order-service/internal/order/service"
	http "github.com/GolangDeveloperAlmir/order-service/internal/order/transport/http"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/auth"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/db"
	server "github.com/GolangDeveloperAlmir/order-service/internal/platform/http"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/idempotency"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/kafka"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/observability"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/outbox"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/saga"
	httpstd "net/http"
	"strings"
)

func Run(ctx context.Context, cfg *config.Config, logger *log.Logger) error {
	observability.InitMetrics()
	shutdownTracer := observability.InitTracing(ctx, logger)
	defer shutdownTracer()

	pool, err := db.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	tx := db.NewTxManager(pool)
	orderRepo := postgres.New(pool)
	orderSvc := service.New(orderRepo, tx)

	idem := idempotency.NewStore(pool)

	prod := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopicOrders)
	defer func() {
		if err := prod.Close(); err != nil {
			logger.Error("failed to close kafka producer", log.Err(err))
		}
	}()

	relay := outbox.New(pool, prod, cfg.OutboxInterval, cfg.OutboxBatch, logger)
	go func() {
		if err := relay.Run(ctx); err != nil {
			return
		}
	}()

	sgStore := saga.NewStore(pool)
	sgMgr := saga.NewManager(sgStore, logger)
	go func() {
		if err := sgMgr.RunPoller(ctx); err != nil {
			return
		}
	}()

	var authMW func(httpstd.Handler) httpstd.Handler
	if cfg.AuthEnabled {
		auds := strings.Split(cfg.OIDCAudience, ",")
		oidcMW, err := auth.NewOIDC(ctx, auth.OIDCConfig{
			Issuer:        cfg.OIDCIssuer,
			Audiences:     auds,
			RequiredScope: cfg.OIDCRequiredScope,
			Logger:        logger,
		})
		if err != nil {
			return fmt.Errorf("oidc init: %w", err)
		}
		authMW = oidcMW.Middleware
	}

	api := http.NewHandler(orderSvc, logger, idem, sgMgr)
	router := http.NewRouter(api, logger, http.WithAuth(authMW))

	srv := server.New(router, cfg, logger)

	return srv.Run(ctx)
}
