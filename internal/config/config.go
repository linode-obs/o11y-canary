package config

import "time"

// TLSConfig represents TLS configuration
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	ServerName         string `yaml:"server_name"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// Endpoint represents an endpoint with optional TLS configuration
type Endpoint struct {
	URL string     `yaml:"url"`
	TLS *TLSConfig `yaml:"tls,omitempty"`
}

// CanaryConfig defines the configuration for a single canary
type CanaryConfig struct {
	Type string `yaml:"type"`
	// give ingest and query endpoints their own endpoint struct for distinct TLS settings but still have global defaults
	TLS              *TLSConfig        `yaml:"tls,omitempty"`
	Ingest           []Endpoint        `yaml:"ingest"`
	Query            []Endpoint        `yaml:"query"`
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
