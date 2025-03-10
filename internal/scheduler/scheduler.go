package scheduler

import (
	"fmt"
	"time"

	"github.com/edgard/murailobot/internal/utils/logging"
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
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	s.Start()

	return &scheduler{scheduler: s}, nil
}

// AddJob adds a new job to the scheduler using a cron expression.
func (s *scheduler) AddJob(name, cronExpr string, job func()) error {
	_, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(job),
		gocron.WithName(name),
	)
	if err != nil {
		logging.Error("failed to add job",
			"name", name,
			"cron", cronExpr,
			"error", err)

		return fmt.Errorf("failed to schedule job %q: %w", name, err)
	}

	logging.Info("job scheduled",
		"name", name,
		"cron", cronExpr)

	return nil
}

// Stop gracefully stops the scheduler.
func (s *scheduler) Stop() error {
	if err := s.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}
