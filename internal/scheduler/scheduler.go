package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
)

var (
	scheduler gocron.Scheduler
	once      sync.Once
	errInit   error
)

// Get the scheduler singleton
//
//nolint:ireturn // Intentionally returning interface type
func getScheduler() (gocron.Scheduler, error) {
	once.Do(func() {
		s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
		if err != nil {
			errInit = fmt.Errorf("failed to create scheduler: %w", err)

			return
		}

		s.Start()
		scheduler = s
	})

	if errInit != nil {
		return nil, errInit
	}

	return scheduler, nil
}

// AddJob adds a new job to the scheduler using a cron expression.
func AddJob(name, cronExpr string, job func()) error {
	s, err := getScheduler()
	if err != nil {
		return fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	_, err = s.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(job),
		gocron.WithName(name),
	)
	if err != nil {
		slog.Error("failed to add job",
			"name", name,
			"cron", cronExpr,
			"error", err)

		return fmt.Errorf("failed to schedule job %q: %w", name, err)
	}

	slog.Info("job scheduled",
		"name", name,
		"cron", cronExpr)

	return nil
}

// Stop gracefully stops the scheduler.
func Stop() error {
	s, err := getScheduler()
	if err != nil {
		return fmt.Errorf("failed to get scheduler for shutdown: %w", err)
	}

	if err := s.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}
