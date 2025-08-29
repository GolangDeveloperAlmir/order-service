package observability

import (
	"context"
	"net/http"

	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InitMetrics is a placeholder for future metrics wiring.
func InitMetrics(ctx context.Context, service string, logger *log.Logger) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

// Handler exposes the Prometheus metrics endpoint.
func Handler() http.Handler { return promhttp.Handler() }
