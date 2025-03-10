package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
)

var (
	scheduler *gocron.Scheduler
	once      sync.Once
)

// getScheduler returns the global scheduler instance.
func getScheduler() *gocron.Scheduler {
	once.Do(func() {
		scheduler = gocron.NewScheduler(time.UTC)
		scheduler.StartAsync()
	})

	return scheduler
}

// AddJob adds a new job to the scheduler using cron expression.
func AddJob(name, cronExpr string, job func()) error {
	s := getScheduler()

	_, err := s.Cron(cronExpr).Name(name).Do(job)
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
		scheduler.Stop()
	}
}
