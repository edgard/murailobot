// Package scheduler defines the interface for scheduling operations.
package scheduler

// JobFunc is a function that will be executed by the scheduler.
type JobFunc func()

// Service defines the interface for scheduler operations.
type Service interface {
	// AddJob adds a new job to the scheduler.
	AddJob(name string, cronSpec string, job JobFunc) error

	// Stop gracefully shuts down the scheduler.
	Stop() error
}
