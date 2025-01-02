package config

import "time"

// CanaryConfig defines the configuration for a single canary
type CanaryConfig struct {
	Type             string            `yaml:"type"`
	Ingest           []string          `yaml:"ingest"`
	Query            []string          `yaml:"query"`
	AdditionalLabels map[string]string `yaml:"additional_labels"`
	Interval         time.Duration     `yaml:"interval"`
}

// CanariesConfig holds multiple canary configurations
type CanariesConfig struct {
	Canaries map[string]CanaryConfig `yaml:"canary"`
}
