package interfaces

import (
	"context"
	"time"
)

// JobInfo holds information about a scheduled job
type JobInfo struct {
	Name      string    // Job name
	JobID     string    // Unique identifier
	NextRun   time.Time // Next scheduled run time
	LastError string    // Last error if any
}

// Scheduler defines the interface for scheduled operations
type Scheduler interface {
	// AddJob adds a new scheduled job
	AddJob(name string, schedule string, job func()) error

	// Start begins scheduler operation
	Start(ctx context.Context) error

	// Stop halts scheduler operation
	Stop() error
}
