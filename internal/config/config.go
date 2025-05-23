package config

import "time"

// CanaryConfig defines the configuration for a single canary
type CanaryConfig struct {
	Type             string            `yaml:"type"`
	Ingest           []string          `yaml:"ingest"`
	Query            []string          `yaml:"query"`
	AdditionalLabels map[string]string `yaml:"additional_labels"`
	Interval         time.Duration     `yaml:"interval"`
	WriteTimeout     time.Duration     `yaml:"write_timeout"`
	QueryTimeout     time.Duration     `yaml:"query_timeout"`
	MaxActiveSeries  int               `yaml:"max_active_canaried_series"` // cardinality limit on maximum active series in rotation
}

// CanariesConfig holds multiple canary configurations
type CanariesConfig struct {
	Canaries map[string]CanaryConfig `yaml:"canary"`
}
