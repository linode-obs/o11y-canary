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
    ingest:
    - metrics-insert.my-cluster.com
    query:
    - select-endpoint.my-cluster.com
    additional_labels:
        environment: staging
    interval: 5m
  my_canary_2:
    type: prometheus
    ingest:
    - metrics-insert.my-cluster.com
    query:
    - select-endpoint.my-cluster.com
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
			Ingest: []string{
				"metrics-insert.my-cluster.com",
			},
			Query: []string{
				"select-endpoint.my-cluster.com",
			},
			AdditionalLabels: map[string]string{
				"environment": "staging",
			},
			Interval: 5 * time.Minute,
		},
		"my_canary_2": {
			Type: "prometheus",
			Ingest: []string{
				"metrics-insert.my-cluster.com",
			},
			Query: []string{
				"select-endpoint.my-cluster.com",
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
}
