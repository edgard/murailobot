package interfaces

import (
	"context"
)

// Scheduler defines the interface for scheduled operations
type Scheduler interface {
	// Configure sets up scheduler with given timezone
	Configure(timezone string) error

	// AddJob adds a new scheduled job
	AddJob(name string, schedule string, job func()) error

	// Start begins scheduler operation
	Start(ctx context.Context) error

	// Stop halts scheduler operation
	Stop() error
}
