package canary

import (
	"context"
	"fmt"
	"log/slog"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"golang.org/x/exp/rand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Monitor is an interface that defines methods for canary operations
type Monitor interface {
	Write()
	Query()
	Publish()
}

// Canary represents a single canary with a monitor and targets
type Canary struct {
	// should we add more values here? ie. targets
	//	m Monitor
	//	t Targets
	// InsertionTimestamps helps keep requestIDs and when they were inserted in order
	InsertionTimestamps sync.Map
}

// Targets holds the canary configurations
type Targets struct {
	// Might make sense to move to Canary, unsure yet
	Config config.CanariesConfig
}

// InitClient method for Canary to provide grpc client, meterprovider (with shutdown func), and metrics for later writing
func (c *Canary) InitClient(ctx context.Context, res *resource.Resource, target string, interval time.Duration, timeout time.Duration) (metric.MeterProvider, func(), metric.Float64Gauge, error) {

	// TODO - TLS support
	// stats handler provides automatic grpc (rpc_) metrics
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create gRPC connection: %v", err)
	}

	// TODO - dynamic CLI flags for connection, target, tls, etc
	meterProvider, err := otelsetup.InitOTLPMeterProvider(ctx, res, conn, timeout)
	if err != nil {
		slog.Error("Failed to create meter provider", "error", err)
		conn.Close()
		return nil, nil, nil, err
	}

	// Return shutdown function for cleanup
	cleanup := func() {
		if shutdownErr := meterProvider.Shutdown(ctx); shutdownErr != nil {
			slog.Error("Failed to shut down meter provider", "target", target, "error", shutdownErr)
		}
		conn.Close()
	}

	canaryMeter := meterProvider.Meter("o11y-canary-exported-data")

	canaryGauge, err := canaryMeter.Float64Gauge(
		"o11y_canary_canaried_metric_total",
		metric.WithDescription("o11y canary test metric for canarying"),
	)

	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create metric for write: %v", err)
	}

	return meterProvider, cleanup, canaryGauge, nil
}

// Write performs a write operation for a counter
func (c *Canary) Write(ctx context.Context, meterProvider metric.MeterProvider, targets []string, gauge metric.Float64Gauge, requestID string) (err error) {
	for _, target := range targets {

		// generate metrics
		randomValue := float64(rand.Intn(100)) // does this need to be random values? i guess why not for later fetching
		// TODO look at loki canary logic again for their values

		// TODO use something like loki canary streams to help identify the time series by labels?
		labels := []attribute.KeyValue{
			attribute.String("target", target),
			attribute.String("canary", "true"),
			// TODO - request_id will be unbounded cardinality
			// need to limit to like 5 active time series or something
			attribute.String("canary_request_id", requestID),
		}

		gauge.Record(ctx, randomValue, metric.WithAttributes(labels...))

		// TODO canonical log here?
		slog.Debug("Writing metric", "ingest", target, "canary_request_id", requestID)

	}
	return nil
}

// Query performs a query operation
func (c *Canary) Query(ctx context.Context, queryTargets []string, requestID string) (err error) {

	for _, target := range queryTargets {
		// https://pkg.go.dev/github.com/prometheus/client_golang@v1.22.0/api/prometheus/v1#example-API-Query
		slog.Debug("Querying metric", "target", target, "canary_request_id", requestID)
		client, err := api.NewClient(api.Config{Address: target})
		if err != nil {
			return err
		}

		api := v1.NewAPI(client)

		query := fmt.Sprintf(`o11y_canary_canaried_metric_total{canary="true", canary_request_id="%s"}`, requestID)

		// discard result, just want to make sure we can query
		_, warnings, err := api.Query(ctx, query, time.Now())
		if err != nil {
			return err
		}
		if len(warnings) > 0 {
			slog.Info("Warning when querying target", "target", target, "canary_request_id", requestID, "warnings", warnings)
		}
		slog.Debug("Query successful", "target", target, "canary_request_id", requestID)
	}
	return nil
}
