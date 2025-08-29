package app

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/GolangDeveloperAlmir/order-service/internal/config"
)

func Run(ctx context.Context, cfg *config.Config, logger *log.Logger) error {
	observability.InitMetrics()
	shutdownTracer := observability.InitTracing(ctx, logger)
	defer shutdownTracer()

	pool, err := db.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to db", "error", err)
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	tx := db.NewTxManager(pool)
	orderRepo := postgres.New(pool)
	orderSvc := service.New(orderRepo, tx)

	idem := idempotency.NewStore(pool)

	prod := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopicOrders)
	defer prod.Close()

	relay := outbox.New(pool, prod, cfg.OutboxInterval, cfg.OutboxBatch, logger)
	go func() {
		if err := relay.Run(ctx); err != nil {
			logger.Error("outbox relay stopped", "error", err)
		}
	}()

	sgStore := saga.NewStore(pool)
	sgMgr := saga.NewManager(sgStore, logger)
	go func() {
		if err := sgMgr.RunPoller(ctx); err != nil {
			logger.Errorf("failed to run poller: %v", err)
		}
	}()

	var authMW func(http.Handler) http.Handler
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
