// Package scheduler provides job scheduling functionality with cron expressions.
package scheduler

import (
	"time"

	"github.com/edgard/murailobot/internal/errs"
	"github.com/edgard/murailobot/internal/logging"
	"github.com/go-co-op/gocron/v2"
)

// New creates and starts a new scheduler instance.
//
//nolint:ireturn // Interface return is intentional for better abstraction
func New() (Scheduler, error) {
	s, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithLogger(logging.NewGocronLogger()),
	)
	if err != nil {
		return nil, errs.NewConfigError("failed to create scheduler", err)
	}

	s.Start()

	return &scheduler{scheduler: s}, nil
}

// AddJob adds a new job to the scheduler using a cron expression.
func (s *scheduler) AddJob(name, cronExpr string, job func()) error {
	if name == "" {
		return errs.NewValidationError("empty job name", nil)
	}

	if cronExpr == "" {
		return errs.NewValidationError("empty cron expression", nil)
	}

	if job == nil {
		return errs.NewValidationError("nil job function", nil)
	}

	_, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(job),
		gocron.WithName(name),
	)
	if err != nil {
		return errs.NewConfigError("failed to schedule job", err)
	}

	return nil
}

// Stop gracefully stops the scheduler.
func (s *scheduler) Stop() error {
	if err := s.scheduler.Shutdown(); err != nil {
		return errs.NewConfigError("failed to shutdown scheduler", err)
	}

	return nil
}
