// Package utils provides utility functions and components for MurailoBot,
// including scheduling, text processing, and other shared functionality.
package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
)

// Scheduler provides job scheduling functionality for recurring tasks
// such as profile updates and message cleanup operations.
type Scheduler struct {
	scheduler gocron.Scheduler
}

// NewScheduler creates and starts a new scheduler instance configured
// to use UTC timezone and structured logging.
//
// Returns an error if the scheduler creation fails.
func NewScheduler() (*Scheduler, error) {
	slog.Debug("creating new scheduler")
	startTime := time.Now()

	s, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithLogger(&gocronLogAdapter{}),
	)
	if err != nil {
		slog.Error("failed to create scheduler",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	slog.Debug("scheduler created, starting scheduler")
	schedulerStartTime := time.Now()

	s.Start()

	startDuration := time.Since(schedulerStartTime)
	totalDuration := time.Since(startTime)

	slog.Debug("scheduler started successfully",
		"start_duration_ms", startDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds())

	return &Scheduler{scheduler: s}, nil
}

// AddJob adds a new job to the scheduler using a cron expression.
// The job will be executed according to the schedule defined by the cron expression.
//
// Parameters:
// - name: A unique identifier for the job
// - cronExpr: A cron expression defining the schedule (e.g., "0 0 * * *" for daily at midnight)
// - job: The function to execute when the job runs
//
// Returns an error if any parameters are invalid or if scheduling fails.
func (s *Scheduler) AddJob(name, cronExpr string, job func()) error {
	slog.Debug("adding scheduled job",
		"job_name", name,
		"cron_expression", cronExpr)

	startTime := time.Now()

	if name == "" {
		slog.Error("failed to add job: empty job name")
		return errors.New("empty job name")
	}

	if cronExpr == "" {
		slog.Error("failed to add job: empty cron expression", "job_name", name)
		return errors.New("empty cron expression")
	}

	if job == nil {
		slog.Error("failed to add job: nil job function", "job_name", name)
		return errors.New("nil job function")
	}

	// Create a wrapper function that adds logging around the job execution
	wrappedJob := func() {
		jobStartTime := time.Now()
		slog.Debug("starting scheduled job",
			"job_name", name,
			"scheduled_at", time.Now().Format(time.RFC3339))

		// Execute the original job function
		job()

		jobDuration := time.Since(jobStartTime)
		slog.Debug("scheduled job completed",
			"job_name", name,
			"duration_ms", jobDuration.Milliseconds())

		// Add Warning for slow job execution
		slowThreshold := 5 * time.Second
		if jobDuration > slowThreshold {
			slog.Warn("slow job execution detected",
				"job_name", name,
				"duration_ms", jobDuration.Milliseconds(),
				"threshold_ms", slowThreshold.Milliseconds())
		}
	}

	scheduledJob, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(wrappedJob),
		gocron.WithName(name),
	)
	if err != nil {
		slog.Error("failed to schedule job",
			"error", err,
			"job_name", name,
			"cron_expression", cronExpr)
		return fmt.Errorf("failed to schedule job: %w", err)
	}

	// Get the next run time to log it
	nextRun, err := scheduledJob.NextRun()
	if err == nil {
		slog.Debug("job scheduled successfully",
			"job_name", name,
			"cron_expression", cronExpr,
			"next_run", nextRun.Format(time.RFC3339),
			"setup_duration_ms", time.Since(startTime).Milliseconds())
	} else {
		slog.Debug("job scheduled successfully",
			"job_name", name,
			"cron_expression", cronExpr,
			"setup_duration_ms", time.Since(startTime).Milliseconds())
	}

	return nil
}

// Stop gracefully shuts down the scheduler and waits for all running jobs
// to complete before returning.
//
// Returns an error if the shutdown process fails.
func (s *Scheduler) Stop() error {
	slog.Debug("stopping scheduler")
	startTime := time.Now()

	// Get the number of jobs before shutdown for logging
	jobs := s.scheduler.Jobs()
	slog.Debug("preparing to shutdown scheduler",
		"active_jobs", len(jobs))

	if err := s.scheduler.Shutdown(); err != nil {
		slog.Error("failed to shutdown scheduler",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	shutdownDuration := time.Since(startTime)
	slog.Debug("scheduler stopped successfully",
		"duration_ms", shutdownDuration.Milliseconds())

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
