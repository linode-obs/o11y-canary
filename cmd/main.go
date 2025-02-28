package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

	slog.Info("Version", "version", Version)
	otelsetup.InitializeResource(Version)

	r := mux.NewRouter()

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

			canaryCtx, canarySpan := tracer.Start(ctx, fmt.Sprintf("canary-%s", name),
				trace.WithAttributes(
					attribute.String("canary.name", name),
					attribute.String("canary.type", canaryConfig.Type),
					attribute.String("ingest.endpoint", canaryConfig.Ingest[0]),
					attribute.Int64("interval_ms", canaryConfig.Interval.Milliseconds()),
					attribute.Int64("timeout_ms", canaryConfig.Timeout.Milliseconds()),
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
			meterProvider, cleanup, gauge, err := c.InitWriteClient(canaryCtx, res, canaryConfig.Ingest[0], canaryConfig.Interval, canaryConfig.Timeout)
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
					canarySpan.AddEvent("Running canary check")

					if canaryConfig.Type == "metrics" {
						// write to ingest
						err := c.Write(canaryCtx, meterProvider, canaryConfig.Ingest, gauge)
						if err != nil {
							errMsg := "Failed to write metrics"
							canarySpan.RecordError(err)
							canarySpan.SetStatus(codes.Error, errMsg)
							canarySpan.AddEvent(errMsg)
							slog.Error(errMsg, "error", err)
						} else {
							canarySpan.AddEvent("Metrics written successfully")
						}

						// query from query URL
						err = c.Query()
						if err != nil {
							errMsg := "Failed to query metrics"
							canarySpan.RecordError(err)
							canarySpan.SetStatus(codes.Error, errMsg)
							canarySpan.AddEvent(errMsg)
							slog.Error(errMsg, "error", err)
						} else {
							canarySpan.AddEvent("Metrics queried successfully")
						}

						// publish results of expected diff (comparator)
						err = c.Publish()
						if err != nil {
							errMsg := "Failed to publish metric query results"
							canarySpan.RecordError(err)
							canarySpan.SetStatus(codes.Error, errMsg)
							canarySpan.AddEvent(errMsg)
							slog.Error(errMsg, "error", err)
						} else {
							canarySpan.AddEvent("Metric query results published successfully")
						}

						canarySpan.End()
					}
				}
			}
		}(canaryName, canaryConfig)
	}
	wg.Wait()

}
