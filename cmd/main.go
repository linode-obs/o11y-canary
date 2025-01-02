package main

import (
	"context"
	"flag"
	"log/slog"
	"o11y-canary/internal/canary"
	"o11y-canary/internal/config"
	"o11y-canary/pkg/otelsetup"
	"os"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	yaml "gopkg.in/yaml.v2"
)

// Version is automatically populated from linker
var Version = "development"

func main() {

	ctx := context.Background()

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

	var config config.CanariesConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		slog.Error("Error decoding YAML", "error", err)
		// TODO - log.fatal equivalent here
	} else {
		slog.Debug("Configuration loaded successfully", "config", config)
	}

	// Set up OpenTelemetry.
	otelShutdown, err := otelsetup.SetupOTelSDK(ctx, Version)
	if err != nil {
		// todo slog fatal?
		slog.Error("Failed to initialize OpenTelemetry", "error", err)
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			slog.Error("Error during OpenTelemetry shutdown", "error", shutdownErr)
		}
	}()

	slog.Info("Version", "version", Version)
	otelsetup.InitializeResource(Version)

	// OTLP provider

	// TODO - concurrency, delete this
	for {
		// for each canary
		for canaryName, canaryConfig := range config.Canaries {
			var c canary.Canary
			if canaryConfig.Type == "metrics" {

				// Initialize the resource for the canary
				res := resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceNameKey.String(canaryName),
					semconv.ServiceNamespaceKey.String("o11y_canary"),
					semconv.ServiceVersionKey.String(Version),
				)

				// TODO - pass through additional labels too
				// write to ingest
				c.Write(ctx, res, canaryConfig.Ingest)

				// query from query URL

				// publish results of expected diff (comparator)

				// repeat on interval
				// TODO - concurrency
				time.Sleep(canaryConfig.Interval)
			}

		}
	}

}
