package observability

import (
	"context"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"time"
)

func InitTracing(ctx context.Context, logger *log.Logger) func() {
	// Deprecated: Use go.opentelemetry.io/otel/trace/noop.NewTracerProvider instead.
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
	return func() {
		logger.Info("tracing shutdown complete", log.Str("ts", time.Now().Format(time.RFC3339)))
	}
}

func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
