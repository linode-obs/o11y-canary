package main

import (
	"context"
	"flag"
	"log/slog"
	"o11y-canary/internal/canary"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
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
	otelShutdown, err := otelsetup.SetupOTelSDK(ctx, Version)
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

	var wg sync.WaitGroup

	for canaryName, canaryConfig := range canaryConfig.Canaries {
		wg.Add(1)
		// Start a goroutine for each canary *run* and handle cancellation
		go func(name string, canaryConfig config.CanaryConfig) {
			defer wg.Done()

			res := resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(canaryName),
				semconv.ServiceNamespaceKey.String(otelsetup.ServiceString),
				semconv.ServiceVersionKey.String(Version),
			)

			var c canary.Canary

			// Initialize client setup outside the ticker loop
			// Each canary gets its own meterProvider (+ grpc client), cleanup func, and single gauge metric
			meterProvider, cleanup, gauge, err := c.InitWriteClient(ctx, res, canaryConfig.Ingest[0], canaryConfig.Interval, canaryConfig.Timeout)
			if err != nil {
				slog.Error("Failed to initialize metric client", "error", err)
				return
			}
			defer cleanup()

			// use ticker for goroutine compatibility
			ticker := time.NewTicker(canaryConfig.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					slog.Info("Shutting down canary", "name", name)
					return
				case <-ticker.C:
					slog.Debug("Running canary", "name", name)

					if canaryConfig.Type == "metrics" {
						// write to ingest
						err := c.Write(ctx, meterProvider, canaryConfig.Ingest, gauge)
						if err != nil {
							slog.Error("Failed to write metrics", "error", err)
						}

						// query from query URL
						err = c.Query()
						if err != nil {
							slog.Error("Failed to query metrics", "error", err)
						}

						// publish results of expected diff (comparator)
						// make channel for publish results?
						err = c.Publish()
						if err != nil {
							slog.Error("Failed to publish metric query results", "error", err)
						}

					}
				}
			}
		}(canaryName, canaryConfig)
	}
	wg.Wait()
}
