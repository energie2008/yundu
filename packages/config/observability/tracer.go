package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func InitTracer(ctx context.Context, serviceName, version string, logger *slog.Logger) (func(context.Context) error, error) {
	exporterType := os.Getenv("OTEL_EXPORTER")
	if exporterType == "" {
		exporterType = "none"
	}

	var exporter sdktrace.SpanExporter
	var err error

	switch exporterType {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
	case "otlp":
		endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if endpoint == "" {
			return nil, fmt.Errorf("OTEL_EXPORTER=otlp requires OTEL_EXPORTER_OTLP_ENDPOINT to be set (e.g. otel-collector:4317)")
		}
		// 解析 endpoint，支持 http:// 或直接 host:port
		endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
		// 简单校验，避免空 host
		if endpoint == "" {
			return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT resolved to empty after trimming scheme")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		if err != nil {
			return nil, fmt.Errorf("create otlp trace exporter (endpoint=%s): %w", endpoint, err)
		}
		logger.Info("otlp trace exporter created", "endpoint", endpoint, "service", serviceName)
	case "none", "":
		logger.Info("tracing disabled (OTEL_EXPORTER=none)")
		return func(context.Context) error { return nil }, nil
	default:
		return nil, fmt.Errorf("unknown OTEL_EXPORTER: %s (supported: stdout, otlp, none)", exporterType)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("tracer initialized", "exporter", exporterType, "service", serviceName)

	return func(shutdownCtx context.Context) error {
		ctx, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(ctx)
	}, nil
}

func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
