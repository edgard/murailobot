// Package cron provides a cron-based implementation of the scheduler service port.
package cron

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/go-co-op/gocron/v2"
)

// cronScheduler implements the scheduler.Service interface
type cronScheduler struct {
	scheduler gocron.Scheduler
}

// NewScheduler creates and starts a new scheduler instance
func NewScheduler() (scheduler.Service, error) {
	s, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithLogger(&gocronLogAdapter{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Start the scheduler
	s.Start()
	slog.Debug("scheduler started")

	return &cronScheduler{scheduler: s}, nil
}

// AddJob adds a new job to the scheduler using a cron expression
func (s *cronScheduler) AddJob(name, cronExpr string, job scheduler.JobFunc) error {
	// Validate parameters
	if name == "" {
		return errors.New("empty job name")
	}

	if cronExpr == "" {
		return errors.New("empty cron expression")
	}

	if job == nil {
		return errors.New("nil job function")
	}

	// Create a wrapper that only logs important information
	wrappedJob := func() {
		// Log start for long jobs only
		var startTime time.Time

		const slowThreshold = 5 * time.Second

		// Using a closure to conditionally time the execution
		func() {
			// Only measure timing for potential logging
			startTime = time.Now()

			// Execute the job
			job()
		}()

		// Only log if the job was slow
		duration := time.Since(startTime)
		if duration > slowThreshold {
			slog.Warn("slow scheduled job execution",
				"job_name", name,
				"duration_ms", duration.Milliseconds())
		}
	}

	// Schedule the job
	scheduledJob, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(wrappedJob),
		gocron.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("failed to schedule job %s: %w", name, err)
	}

	// Get the next run time and log in a single statement
	logAttrs := []interface{}{"job_name", name, "cron", cronExpr}

	if nextRun, err := scheduledJob.NextRun(); err == nil {
		logAttrs = append(logAttrs, "next_run", nextRun.Format(time.RFC3339))
	}

	slog.Info("job scheduled", logAttrs...)

	return nil
}

// Stop gracefully shuts down the scheduler
func (s *cronScheduler) Stop() error {
	slog.Debug("stopping scheduler", "active_jobs", len(s.scheduler.Jobs()))

	if err := s.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

type gocronLogAdapter struct{}

func (l *gocronLogAdapter) Debug(msg string, args ...interface{}) {
	slog.Debug(msg, toSlogArgs(args)...)
}

func (l *gocronLogAdapter) Info(msg string, args ...interface{}) {
	slog.Info(msg, toSlogArgs(args)...)
}

func (l *gocronLogAdapter) Warn(msg string, args ...interface{}) {
	slog.Warn(msg, toSlogArgs(args)...)
}

func (l *gocronLogAdapter) Error(msg string, args ...interface{}) {
	slog.Error(msg, toSlogArgs(args)...)
}

func toSlogArgs(args []interface{}) []interface{} {
	slogArgs := make([]interface{}, 0, len(args))

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key, ok := args[i].(string)
			if !ok {
				key = fmt.Sprintf("%v", args[i])
			}

			slogArgs = append(slogArgs, key, args[i+1])
		} else {
			slogArgs = append(slogArgs, "value", args[i])
		}
	}

	return slogArgs
}
