package canary

import "o11y-canary/internal/config"

// Monitor is an interface that defines methods for canary operations
type Monitor interface {
	Write()
	Query()
	Publish()
}

// Canary represents a single canary with a monitor and targets
type Canary struct {
	// should we add more values here? ie. targets
	//	m Monitor
	//	t Targets
}

// Targets holds the canary configurations
type Targets struct {
	// Might make sense to move to Canary, unsure yet
	Config config.CanariesConfig
}

// Write performs a write operation
func (c *Canary) Write() {
}

// Query performs a query operation
func (c *Canary) Query() {
}

// Publish writes out new values to the /metrics interface.
func (c *Canary) Publish() {
	// method to write out new values to the /metrics interface
	// perhaps return meter type?
	// https://pkg.go.dev/go.opentelemetry.io/otel/metric#MeterProvider
}
