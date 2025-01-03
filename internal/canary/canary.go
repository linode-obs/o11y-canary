package canary

import (
	"context"
	"fmt"
	"log/slog"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"

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
}

// Targets holds the canary configurations
type Targets struct {
	// Might make sense to move to Canary, unsure yet
	Config config.CanariesConfig
}

// Write performs a write operation
// TODO look at loki canary logic again
// https://github.com/grafana/loki/blob/main/pkg/canary/writer/push.go
func (c *Canary) Write(ctx context.Context, res *resource.Resource, targets []string) (err error) {

	// get address to write to from config
	for _, target := range targets {

		// best place to make the conn is here?
		// Create a gRPC connection to the OTLP endpoint

		// TODO TLS support
		conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("Failed to create gRPC connection: %v", err)
		}
		defer conn.Close()

		// TODO - dynamic CLI flags for connection, target, tls, etc
		meterProvider, err := otelsetup.InitOTLPMeterProvider(ctx, res, conn)
		if err != nil {
			slog.Error("Failed to create meter provider", "error", err)
		}
		// now use provider to make new metrics

		meter := meterProvider.Meter("todo_change_this_to_canary_name")
		counter, err := meter.Int64Counter(
			"o11y_canary_canaried_metric_total",
			metric.WithDescription("o11y canary test metric for canarying"),
		)
		defer func() {
			if shutdownErr := meterProvider.Shutdown(ctx); shutdownErr != nil {
				slog.Error("Failed to shut down meter provider", "target", target, "error", shutdownErr)
			}
		}()

		if err != nil {
			return fmt.Errorf("Failed to create metric for write: %v", err)
		}

		// generate metrics
		randomValue := int64(rand.Intn(100)) // random data for now, probably don't need to randomize the value just yet?
		// look at loki canary logic again

		// todo use something like loki canary streams to help identify the time series by labels?
		labels := []attribute.KeyValue{
			attribute.String("target", target),
			attribute.String("canary", "true"),
		}

		// Record the metric
		counter.Add(ctx, randomValue, metric.WithAttributes(labels...))

		slog.Debug("Writing metric", "ingest", target)

	}
	return nil
}

// Query performs a query operation
func (c *Canary) Query() (err error) {

	return nil
}

// Publish writes out new values to the /metrics interface.
func (c *Canary) Publish() (err error) {
	// method to write out new values to the /metrics interface
	// perhaps return meter type?
	// https://pkg.go.dev/go.opentelemetry.io/otel/metric#MeterProvider

	return nil
}
