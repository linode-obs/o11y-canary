package canary

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"
	"os"
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
	"google.golang.org/grpc/credentials"
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
	// ActiveRequestIDs is a list of request IDs that are currently active. Used to limit cardinality
	ActiveRequestIDs []string
}

// Targets holds the canary configurations
type Targets struct {
	// Might make sense to move to Canary, unsure yet
	Config config.CanariesConfig
}

// InitClient method for Canary to provide grpc client, meterprovider (with shutdown func), and metrics for later writing
func (c *Canary) InitClient(ctx context.Context, res *resource.Resource, target string, interval time.Duration, timeout time.Duration, tlsConfig *config.TLSConfig) (metric.MeterProvider, func(), metric.Float64Gauge, error) {

	// spent a while looking at TLS Implementations, easiest to just reload on each new connection
	var creds credentials.TransportCredentials
	if tlsConfig != nil && tlsConfig.Enabled {
		tlsConf := &tls.Config{
			ServerName:         tlsConfig.ServerName,
			InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}

		if tlsConfig.CertFile != "" && tlsConfig.KeyFile != "" {
			tlsConf.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
				if err != nil {
					return nil, fmt.Errorf("failed to load client certificates: %w", err)
				}
				return &cert, nil
			}
		}

		if tlsConfig.CAFile != "" {
			caCert, err := os.ReadFile(tlsConfig.CAFile)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to read CA file: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, nil, nil, fmt.Errorf("failed to parse CA certificate")
			}
			tlsConf.RootCAs = caCertPool
		}

		creds = credentials.NewTLS(tlsConf)
	} else {
		creds = insecure.NewCredentials()
	}

	// stats handler provides automatic grpc (rpc_) metrics
	slog.Debug("Setting up gRPC client", "target", target, "tls_enabled", tlsConfig != nil && tlsConfig.Enabled)
	if tlsConfig != nil && tlsConfig.Enabled {
		slog.Debug("gRPC TLS config", "server_name", tlsConfig.ServerName, "insecure_skip_verify", tlsConfig.InsecureSkipVerify, "cert_file", tlsConfig.CertFile, "key_file", tlsConfig.KeyFile, "ca_file", tlsConfig.CAFile)
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(creds),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		slog.Error("Failed to create gRPC connection", "target", target, "error", err)
		return nil, nil, nil, fmt.Errorf("failed to create gRPC connection: %v", err)
	}
	slog.Debug("gRPC client connection established", "target", target)

	// TODO - dynamic CLI flags for connection, target, etc
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
// Only records the canaried metric (o11y_canary_canaried_metric_total) via the OTLP meter
func (c *Canary) Write(ctx context.Context, meterProvider metric.MeterProvider, targets []string, gauge metric.Float64Gauge, requestID string, writeTimeout time.Duration, wg *sync.WaitGroup) (err error) {
	defer wg.Done()
	done := make(chan error, 1)
	go func() {
		for _, target := range targets {

			// generate metrics
			randomValue := float64(rand.Intn(100)) // does this need to be random values? i guess why not for later fetching
			// TODO look at loki canary logic again for their values

			// TODO use something like loki canary streams to help identify the time series by labels?
			labels := []attribute.KeyValue{
				attribute.String("target", target),
				attribute.String("canary", "true"),
				attribute.String("canary_request_id", requestID),
			}

			gauge.Record(ctx, randomValue, metric.WithAttributes(labels...))

			slog.Debug("Writing canaried metric", "ingest", target, "canary_request_id", requestID)

			// Force flush metrics after recording
			if flusher, ok := meterProvider.(interface{ ForceFlush(context.Context) error }); ok {
				err := flusher.ForceFlush(ctx)
				if err != nil {
					slog.Error("Failed to force flush metrics", "error", err)
				} else {
					slog.Debug("Force flush succeeded", "canary_request_id", requestID)
				}
			} else {
				slog.Warn("MeterProvider does not support ForceFlush")
			}

			// TODO - return error and metrics for failed writes better. also return error + metric for timeouts

		}
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(writeTimeout):
		err := fmt.Errorf("write operation timed out after %s", writeTimeout)
		slog.Error("Write timeout", "canary_request_id", requestID, "timeout", writeTimeout)
		c.InsertionTimestamps.Store(requestID, err)
		return err
	}
}

// Query performs a query operation
func (c *Canary) Query(ctx context.Context, queryTargets []string, requestID string, queryTimeout time.Duration, tlsConfig *config.TLSConfig, wg *sync.WaitGroup) (err error) {
	defer wg.Done()
	done := make(chan error, 1)
	go func() {
		var lastErr error
		for _, target := range queryTargets {
			slog.Debug("Querying metric", "target", target, "canary_request_id", requestID)

			clientConfig := api.Config{Address: target}

			if tlsConfig != nil && tlsConfig.Enabled {
				tlsClientConfig := &tls.Config{
					ServerName:         tlsConfig.ServerName,
					InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
					MinVersion:         tls.VersionTLS12,
				}

				if tlsConfig.CertFile != "" && tlsConfig.KeyFile != "" {
					tlsClientConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
						cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
						if err != nil {
							return nil, fmt.Errorf("failed to load client certificates: %w", err)
						}
						return &cert, nil
					}
				}

				if tlsConfig.CAFile != "" {
					caCert, err := os.ReadFile(tlsConfig.CAFile)
					if err != nil {
						done <- fmt.Errorf("failed to read CA file: %w", err)
						return
					}
					caCertPool := x509.NewCertPool()
					if !caCertPool.AppendCertsFromPEM(caCert) {
						done <- fmt.Errorf("failed to parse CA certificate")
						return
					}
					tlsClientConfig.RootCAs = caCertPool
				}

				clientConfig.RoundTripper = &http.Transport{
					TLSClientConfig: tlsClientConfig,
				}
			}

			client, err := api.NewClient(clientConfig)
			if err != nil {
				done <- err
				return
			}

			api := v1.NewAPI(client)

			query := fmt.Sprintf(`o11y_canary_canaried_metric_total{canary="true", canary_request_id="%s"}`, requestID)

			// Apply per-query timeout via context
			queryCtx, cancel := context.WithTimeout(ctx, queryTimeout*time.Second)
			defer cancel()

			// discard result, just want to make sure we can query
			result, warnings, err := api.Query(queryCtx, query, time.Now())
			if err != nil {
				lastErr = err
				continue
			}
			if len(warnings) > 0 {
				slog.Info("Warning when querying target", "target", target, "canary_request_id", requestID, "warnings", warnings)
			}
			if result == nil || result.String() == "" {
				slog.Warn("Metric not found in query result", "target", target, "canary_request_id", requestID)
				lastErr = fmt.Errorf("metric not found in query result for target %s with request ID %s", target, requestID)
				continue
			}
			slog.Debug("Query successful", "target", target, "canary_request_id", requestID, "result", result.String())

			// TODO - return error and metrics for failed writes better. also return error + metric for timeouts
		}
		done <- lastErr
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(queryTimeout):
		err := fmt.Errorf("query operation timed out after %s", queryTimeout)
		c.InsertionTimestamps.Store(requestID, err)
		slog.Error("Query timeout", "canary_request_id", requestID, "timeout", queryTimeout)
		return err
	case <-done:
		return nil
	}

}
