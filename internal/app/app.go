package app

import (
	"context"
	"crypto/tls"
	"errors"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	httpstd "net/http"
	pprof "net/http/pprof"
	"strings"
	"time"
)

func Run(ctx context.Context, cfg *config.Config, logger *log.Logger) error {
	metricsShutdown, err := observability.InitMetrics(ctx, "order-service", logger)
	if err != nil {
		return fmt.Errorf("metrics init: %w", err)
	}
	defer func() {
		if err := metricsShutdown(context.Background()); err != nil {
			logger.Error("metrics shutdown", log.Err(err))
		}
	}()

	tracerShutdown, err := observability.InitTracing(ctx, "order-service", logger)
	if err != nil {
		return fmt.Errorf("tracing init: %w", err)
	}
	defer func() {
		if err := tracerShutdown(context.Background()); err != nil {
			logger.Error("tracing shutdown", log.Err(err))
		}
	}()

	pool, err := db.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	tx := db.NewTxManager(pool)
	orderRepo := postgres.New(pool)
	orderSvc := service.New(orderRepo, tx, logger)

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
	router = otelhttp.NewHandler(router, "http.api")

	debugMux := httpstd.NewServeMux()
	debugMux.Handle("/metrics", observability.Handler())
	debugMux.HandleFunc("/debug/pprof/", pprof.Index)
	debugMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	debugMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	debugMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	debugMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	debugSrv := &httpstd.Server{
		Addr:              cfg.DebugAddr,
		Handler:           debugMux,
		ReadHeaderTimeout: cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	go func() {
		logger.Info("debug server started", log.Str("addr", debugSrv.Addr))
		if err := debugSrv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && !errors.Is(err, httpstd.ErrServerClosed) {
			logger.Error("debug server error", log.Err(err))
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := debugSrv.Shutdown(ctx); err != nil {
			logger.Error("debug shutdown error", log.Err(err))
		}
	}()

	srv := server.New(router, cfg, logger)

	return srv.Run(ctx)
}
