// Package scheduler defines the port interface for scheduling functionality
package scheduler

// Service defines the interface for scheduling recurring tasks and jobs
type Service interface {
	// AddJob schedules a new job with the given name and cron expression
	AddJob(name, cronExpr string, job func()) error

	// Stop gracefully shuts down the scheduler
	Stop() error
}
