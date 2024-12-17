package config_test

import (
	"o11y-canary/internal/config"
	"reflect"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestSingleCanaryConfig(t *testing.T) {
	// Ensure canary config is parsed correctly for one canary
	// TODO convert to table test
	// TODO test multiple canaries in the same config file
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
    `
	var config config.CanariesConfig

	err := yaml.Unmarshal([]byte(yamlInput), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	expectedType := "otlp"
	expectedIngest := "metrics-insert.my-cluster.com"
	expectedQuery := "select-endpoint.my-cluster.com"
	expectedAdditionalLabels := map[string]string{
		"environment": "staging",
	}

	if config.Canaries["my_canary_1"].Type != expectedType {
		t.Errorf("Expected '%s', got '%s'", expectedType, config.Canaries["my_canary_1"].Type)
	}

	if config.Canaries["my_canary_1"].Ingest[0] != expectedIngest {
		t.Errorf("Expected '%s', got '%s'", expectedIngest, config.Canaries["my_canary_1"].Ingest[0])
	}

	if config.Canaries["my_canary_1"].Query[0] != expectedQuery {
		t.Errorf("Expected '%s', got '%s'", expectedQuery, config.Canaries["my_canary_1"].Query[0])
	}

	if !reflect.DeepEqual(config.Canaries["my_canary_1"].AdditionalLabels, expectedAdditionalLabels) {
		t.Errorf("Expected '%s', got '%s'", expectedAdditionalLabels, config.Canaries["my_canary_1"].AdditionalLabels)
	}

}
