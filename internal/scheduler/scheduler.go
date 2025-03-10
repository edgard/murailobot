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
)

// Get the scheduler singleton
//
//nolint:ireturn // Intentionally returning interface type
func getScheduler() gocron.Scheduler {
	once.Do(func() {
		s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
		if err != nil {
			panic(fmt.Sprintf("failed to create scheduler: %v", err))
		}

		s.Start()
		scheduler = s
	})

	return scheduler
}

// AddJob adds a new job to the scheduler using a cron expression.
func AddJob(name, cronExpr string, job func()) error {
	s := getScheduler()

	_, err := s.NewJob(
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
func Stop() {
	if scheduler != nil {
		if err := scheduler.Shutdown(); err != nil {
			slog.Error("error shutting down scheduler", "error", err)
		}
	}
}
