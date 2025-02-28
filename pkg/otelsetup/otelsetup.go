package otelsetup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"google.golang.org/grpc"
)

// ServiceString is for opentelemetry information
var ServiceString = "o11y-canary"

// Meter is a package wide service description variable for metrics
var Meter = otel.Meter(ServiceString)

// Tracer is a package wide service description variable for traces
var Tracer = otel.Tracer(ServiceString)

// SetupOTelSDK implements various telemetry providers for the o11y-canary itself
func SetupOTelSDK(ctx context.Context, version string, tracingEndpoint string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// Shutdown function for cleanup
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Handle errors and call shutdown
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Initialize Resource
	res := InitializeResource(version)

	// Set up meter provider
	meterProvider, err := newMeterProvider(res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Setup tracing only if tracingEndpoint is provided
	if tracingEndpoint != "" {
		slog.Info("Enabling tracing", "tracingEndpoint", tracingEndpoint)

		var tp *trace.TracerProvider
		var shutdownTracing func(context.Context) error
		tp, shutdownTracing, err = setupTracing(ctx, res, tracingEndpoint)
		// TODO look at shutdownfuncs
		if err != nil {
			handleErr(err)
			return
		}
		shutdownFuncs = append(shutdownFuncs, shutdownTracing)
		otel.SetTracerProvider(tp)
	}

	return
}

// InitOTLPMeterProvider initializes an OTLP exporter, and configures the corresponding meter provider for canaries
// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/examples/otel-collector/main.go
func InitOTLPMeterProvider(ctx context.Context, res *resource.Resource, conn *grpc.ClientConn, timeout time.Duration) (*metric.MeterProvider, error) {
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}

	// small buffer to prevent thundering herd as "The collect and export time are not counted towards the interval between attempts."
	buffer := 5 * time.Second
	interval := timeout + buffer

	// PeriodicReader helps prevent managing forceflush by hand
	reader := metric.NewPeriodicReader(
		metricExporter,
		metric.WithInterval(interval),
		metric.WithTimeout(timeout),
	)

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)

	return meterProvider, nil
}

func setupTracing(ctx context.Context, res *resource.Resource, endpoint string) (*trace.TracerProvider, func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	return tracerProvider, tracerProvider.Shutdown, nil
}

// InitializeResource initializes the resource with the given version as we have to link that from main
func InitializeResource(version string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(ServiceString), // Explicit service name
		semconv.ServiceNamespaceKey.String(ServiceString),
		semconv.ServiceVersionKey.String(version),
	)
}

func newMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithResource(res),
	)
	return provider, nil
}
