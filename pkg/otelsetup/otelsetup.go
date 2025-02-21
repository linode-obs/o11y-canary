package otelsetup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"google.golang.org/grpc"
)

// ServiceString is for opentelemetry information
var ServiceString = "o11y-canary"

// Meter is a package wide service description variable for metrics
var Meter = otel.Meter(ServiceString)

// Tracer is a package wide service description variable for traces
var Tracer = otel.Tracer(ServiceString)

// SetupOTelSDK implements various telemetry providers for the o11y-canary itself, not any subsequent canary instructions
func SetupOTelSDK(ctx context.Context, version string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// dynamic version from linker
	res := InitializeResource(version)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

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
