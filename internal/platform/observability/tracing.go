package observability

import (
	"context"
	"os"

	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracing configures OpenTelemetry tracing with an OTLP exporter if
// OTEL_EXPORTER_OTLP_ENDPOINT is provided, otherwise falling back to stdout.
func InitTracing(ctx context.Context, service string, logger *log.Logger) (func(context.Context) error, error) {
	var exp sdktrace.SpanExporter
	var err error
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		exp, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	} else {
		exp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		logger.Error("trace exporter", log.Err(err))
		return nil, err
	}

	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(semconv.ServiceName(service)))
	if err != nil {
		logger.Error("trace resource", log.Err(err))
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// Tracer returns a named tracer.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
