package scheduler

import (
	"github.com/go-co-op/gocron/v2"
)

// Scheduler provides an interface for scheduling and managing jobs.
type Scheduler interface {
	// AddJob adds a new job to the scheduler using a cron expression.
	AddJob(name, cronExpr string, job func()) error

	// Stop gracefully stops the scheduler.
	Stop() error
}

// scheduler implements the Scheduler interface.
type scheduler struct {
	scheduler gocron.Scheduler
}
