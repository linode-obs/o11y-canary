package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"o11y-canary/internal/canary"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	yaml "gopkg.in/yaml.v2"
)

// Version is automatically populated from linker
var Version = "development"

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultLogLevel := "info"

	logLevel := flag.String("log.level", defaultLogLevel, "Set log level (options: info, warn, error, debug)")
	configFileFlag := flag.String("config", "config.yaml", "Path to the configuration file")
	tracingEndpoint := flag.String("tracing.endpoint", "localhost:4317", "Tracing endpoint")
	flag.Parse()

	var slogLevel slog.Level
	switch *logLevel {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})

	slog.SetDefault(slog.New(handler))
	slog.Info("Logger initialized", "level", *logLevel)

	file, err := os.Open(*configFileFlag)
	if err != nil {
		slog.Error("Error decoding YAML", "error", err)
	}
	defer file.Close()

	var canaryConfig config.CanariesConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&canaryConfig); err != nil {
		slog.Error("Error decoding YAML", "error", err)
		os.Exit(1)
	} else {
		slog.Debug("Configuration loaded successfully", "config", canaryConfig)
	}

	// apply defaults to config if they are not set
	for name := range canaryConfig.Canaries {
		config := canaryConfig.Canaries[name]

		if config.MaxActiveSeries == 0 {
			config.MaxActiveSeries = 50
		}
		if config.Interval == 0 {
			config.Interval = 5 * time.Second
		}
		if config.WriteTimeout == 0 {
			config.WriteTimeout = 10 * time.Second
		}
		if config.QueryTimeout == 0 {
			config.QueryTimeout = 60 * time.Second
		}
		if config.Type == "" {
			config.Type = "metrics"
		}

		canaryConfig.Canaries[name] = config
	}

	// Set up OpenTelemetry.
	otelShutdown, err := otelsetup.SetupOTelSDK(ctx, Version, *tracingEndpoint)
	if err != nil {
		slog.Error("Failed to initialize OpenTelemetry", "error", err)
		os.Exit(1)
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			slog.Error("Error during OpenTelemetry shutdown", "error", shutdownErr)
		}
	}()

	slog.Info("Service info",
		"version", Version,
		"log_level", *logLevel,
		"config_file", *configFileFlag,
		"tracing_endpoint", *tracingEndpoint,
		"service.name", "o11y-canary",
		"service.version", Version,
		"service.namespace", otelsetup.ServiceString,
	)

	otelsetup.InitializeResource(Version)

	r := mux.NewRouter()

	// internal metric setup
	promExporter, err := otelprom.New(otelprom.WithRegisterer(prometheus.DefaultRegisterer))
	if err != nil {
		log.Fatalf("failed to create Prometheus exporter: %v", err)
	}

	promMeterProvider := otelmetric.NewMeterProvider(
		otelmetric.WithReader(promExporter),
	)

	// this meter is for internal metrics
	meter := promMeterProvider.Meter("o11y-canary")

	infoGauge, _ := meter.Float64Gauge(
		"o11y_canary_info",
		metric.WithDescription("o11y canary information"),
	)

	infoGauge.Record(ctx, 1, metric.WithAttributes(
		attribute.String("version", Version),
		attribute.String("log_level", *logLevel),
		attribute.String("config_file", *configFileFlag),
		attribute.String("tracing_endpoint", *tracingEndpoint),
		attribute.String("service.name", "o11y-canary"),
		attribute.String("service.version", Version),
		attribute.String("service.namespace", otelsetup.ServiceString),
	))

	// pprof boilerplate
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	r.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	r.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")

	go func() {
		slog.Info("Starting metrics & profiling http server", "port", 8080)
		if err := http.ListenAndServe(":8080", r); err != nil {
			slog.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// canonical trace
	tracer := otel.Tracer("o11y-canary")
	ctx, span := tracer.Start(ctx, "main",
		trace.WithAttributes(
			attribute.String("tracing_endpoint", *tracingEndpoint),
			attribute.String("service.name", "o11y-canary"),
			attribute.String("service.version", Version),
		),
	)
	span.AddEvent("Service started")
	span.SetAttributes(
		attribute.String("config_file", *configFileFlag),
		attribute.String("log_level", *logLevel),
	)
	defer span.End()

	var wg sync.WaitGroup

	for canaryName, canaryConfig := range canaryConfig.Canaries {
		wg.Add(1)
		// Start a goroutine for each canary *run* and handle cancellation
		go func(name string, canaryConfig config.CanaryConfig) {
			defer wg.Done()

			tracer := otel.Tracer("o11y-canary")

			canaryCtx, canarySpan := tracer.Start(ctx, fmt.Sprintf("canary-%s", name),
				trace.WithAttributes(
					attribute.String("canary.name", name),
					attribute.String("canary.type", canaryConfig.Type),
					attribute.String("ingest.endpoint", canaryConfig.Ingest[0]),
					attribute.Int64("interval_ms", canaryConfig.Interval.Milliseconds()),
					attribute.Int64("write_timeout_ms", canaryConfig.WriteTimeout.Microseconds()),
					attribute.Int64("query_timeout_ms", canaryConfig.QueryTimeout.Microseconds()),
				),
			)
			canarySpan.AddEvent("Canary initialized")

			res := resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(canaryName),
				semconv.ServiceNamespaceKey.String(otelsetup.ServiceString),
				semconv.ServiceVersionKey.String(Version),
			)

			var c canary.Canary

			// Initialize client setup outside the ticker loop
			// Each canary gets its own meterProvider (+ grpc client), cleanup func, and single gauge metric
			// TODO handle multiples ingests
			meterProvider, cleanup, gauge, err := c.InitClient(canaryCtx, res, canaryConfig.Ingest[0], canaryConfig.Interval, canaryConfig.WriteTimeout)
			if err != nil {
				errMsg := "Failed to initialize metric client"
				canarySpan.RecordError(err)
				canarySpan.SetStatus(codes.Error, errMsg)
				canarySpan.AddEvent(errMsg)
				slog.Error(errMsg, "error", err)
				canarySpan.End()
				return
			}
			defer cleanup()

			// use ticker for goroutine compatibility
			ticker := time.NewTicker(canaryConfig.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					infoMsg := "Canary shutdown after context cancellation"
					canarySpan.SetStatus(codes.Ok, infoMsg)
					canarySpan.AddEvent(infoMsg)
					slog.Info(infoMsg, "name", name)
					canarySpan.End()
					return

				case <-ticker.C:
					// start a new trace for each canary op
					runCtx, runSpan := tracer.Start(canaryCtx, fmt.Sprintf("canary-write-%s", name))
					runSpan.AddEvent("Running canary check")

					if canaryConfig.Type == "metrics" {
						var requestID string
						if len(c.ActiveRequestIDs) < canaryConfig.MaxActiveSeries {
							requestID = runSpan.SpanContext().SpanID().String()
							c.ActiveRequestIDs = append(c.ActiveRequestIDs, requestID)
						} else {
							// re-use oldest requestID
							requestID = c.ActiveRequestIDs[0]
							// then rotate the requestID to the end of the list
							c.ActiveRequestIDs = append(c.ActiveRequestIDs[1:], requestID)
						}

						// write to ingest
						insertionTime := time.Now()
						// align requestID with insertion time for later diffing
						c.InsertionTimestamps.Store(requestID, insertionTime)
						err := c.Write(runCtx, meterProvider, canaryConfig.Ingest, gauge, requestID)
						if err != nil {
							runSpan.RecordError(err)
							runSpan.SetStatus(codes.Error, "Failed to write metrics")
							slog.Error("Failed to write metrics", "error", err)
						} else {
							runSpan.AddEvent("Metrics written successfully")
						}

						// total + success + error is a bit verbose but comfortable
						queriesTotal, _ := meter.Int64Counter(
							"o11y_canary_queries_total",
							metric.WithDescription("Total number of query attempts, including success and failures"),
						)

						querySuccesses, _ := meter.Int64Counter(
							"o11y_canary_query_successes_total",
							metric.WithDescription("Total number of successful queries"),
						)

						queryErrors, _ := meter.Int64Counter(
							"o11y_canary_query_errors_total",
							metric.WithDescription("Total number of failed queries"),
						)

						// initiate error metric in case errors are rare
						// TODO - Prometheus created timestamp would be better
						queryErrors.Add(ctx, 0, metric.WithAttributes(
							attribute.String("canary_name", name),
						))

						durationHistogram, _ := meter.Float64Histogram(
							"o11y_canary_query_duration_seconds",
							metric.WithDescription("Duration of successful queries"),
							metric.WithUnit("s"),
							metric.WithExplicitBucketBoundaries(0.01, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30),
						)

						lagHistogram, _ := meter.Float64Histogram(
							"o11y_canary_lag_duration_seconds",
							metric.WithDescription("Duration of how long metric takes to populate from write to query"),
							metric.WithUnit("s"),
							metric.WithExplicitBucketBoundaries(0.01, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30),
						)

						start := time.Now()
						queriesTotal.Add(ctx, 1, metric.WithAttributes(
							attribute.String("canary_name", name),
						))

						err = c.Query(runCtx, canaryConfig.Query, requestID, canaryConfig.QueryTimeout)
						if err != nil {
							runSpan.RecordError(err)
							runSpan.SetStatus(codes.Error, "Failed to query metrics")
							slog.Error("Failed to query metrics", "error", err)
							queryErrors.Add(ctx, 1, metric.WithAttributes(
								attribute.String("canary_name", name),
							))
						} else {
							duration := time.Since(start).Seconds()
							querySuccesses.Add(ctx, 1, metric.WithAttributes(
								attribute.String("canary_name", name),
							))
							durationHistogram.Record(ctx, duration, metric.WithAttributes(
								attribute.String("canary_name", name),
							))

							if val, ok := c.InsertionTimestamps.Load(requestID); ok {
								// must do type assertion of insertedTime
								insertedAt := val.(time.Time)
								lag := time.Since(insertedAt).Seconds()
								lagHistogram.Record(runCtx, lag, metric.WithAttributes(
									attribute.String("canary_name", name),
								))
								// clean up the map, can't hurt
								c.InsertionTimestamps.Delete(requestID)
							}

							runSpan.AddEvent("Metrics queried successfully")
						}

						runSpan.End()

					}
					canarySpan.End()
				}

			}
		}(canaryName, canaryConfig)
	}
	wg.Wait()
}
