package config_test

import (
	"o11y-canary/internal/config"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

func TestCanariesConfig(t *testing.T) {
	yamlInput := `
canary:
  my_canary_1:
    type: otlp
    tls:
      enabled: true
      ca_file: /etc/ca.crt
      cert_file: /etc/client.crt
      key_file: /etc/client.key
      server_name: collector
    ingest:
      - url: metrics-insert.my-cluster.com
      - url: metrics-insert-2.my-cluster.com
        tls:
          enabled: true
          ca_file: /etc/other-ca.crt
          cert_file: /etc/other-client.crt
          key_file: /etc/other-client.key
          server_name: other-collector
    query:
      - url: select-endpoint.my-cluster.com
    additional_labels:
      environment: staging
    interval: 5m
  my_canary_2:
    type: prometheus
    ingest:
      - url: metrics-insert.my-cluster.com
    query:
      - url: select-endpoint.my-cluster.com
    additional_labels:
      environment: production
    interval: 10m
`
	var canaryConfig config.CanariesConfig

	err := yaml.Unmarshal([]byte(yamlInput), &canaryConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	expectedCanaries := map[string]config.CanaryConfig{
		"my_canary_1": {
			Type: "otlp",
			TLS: &config.TLSConfig{
				Enabled:    true,
				CAFile:     "/etc/ca.crt",
				CertFile:   "/etc/client.crt",
				KeyFile:    "/etc/client.key",
				ServerName: "collector",
			},
			Ingest: []config.Endpoint{
				{
					URL: "metrics-insert.my-cluster.com",
					TLS: nil, // should inherit from canary-level TLS
				},
				{
					URL: "metrics-insert-2.my-cluster.com",
					TLS: &config.TLSConfig{
						Enabled:    true,
						CAFile:     "/etc/other-ca.crt",
						CertFile:   "/etc/other-client.crt",
						KeyFile:    "/etc/other-client.key",
						ServerName: "other-collector",
					},
				},
			},
			Query: []config.Endpoint{
				{URL: "select-endpoint.my-cluster.com"},
			},
			AdditionalLabels: map[string]string{
				"environment": "staging",
			},
			Interval: 5 * time.Minute,
		},
		"my_canary_2": {
			Type: "prometheus",
			Ingest: []config.Endpoint{
				{URL: "metrics-insert.my-cluster.com"},
			},
			Query: []config.Endpoint{
				{URL: "select-endpoint.my-cluster.com"},
			},
			AdditionalLabels: map[string]string{
				"environment": "production",
			},
			Interval: 10 * time.Minute,
		},
	}

	if !reflect.DeepEqual(canaryConfig.Canaries, expectedCanaries) {
		t.Errorf("Expected '%v', got '%v'", expectedCanaries, canaryConfig.Canaries)
	}

	// endpoints without their own TLS should inherit the canary-level TLS
	c1 := canaryConfig.Canaries["my_canary_1"]
	if c1.Ingest[0].TLS != nil {
		t.Errorf("Expected ingest[0] to have nil TLS (should inherit from canary), got %+v", c1.Ingest[0].TLS)
	}
	if c1.Ingest[1].TLS == nil || c1.Ingest[1].TLS.CAFile != "/etc/other-ca.crt" {
		t.Errorf("Expected ingest[1] to have its own TLS config, got %+v", c1.Ingest[1].TLS)
	}
}
