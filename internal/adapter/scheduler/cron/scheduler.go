// Package scheduler provides an implementation of the scheduler port interface
// using the gocron library.
package cron

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
	"go.uber.org/zap"
)

// Scheduler provides job scheduling functionality for recurring tasks
// using the gocron library.
type Scheduler struct {
	scheduler gocron.Scheduler
	logger    *zap.Logger
}

// NewScheduler creates and starts a new scheduler instance configured
// to use UTC timezone and structured logging.
//
// Returns an error if the scheduler creation fails.
func NewScheduler(logger *zap.Logger) (*Scheduler, error) {
	// Create logger adapter with debug level checking
	gocronLogger := &gocronLogAdapter{logger: logger}

	s, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithLogger(gocronLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Start the scheduler
	s.Start()

	return &Scheduler{
		scheduler: s,
		logger:    logger,
	}, nil
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
			s.logger.Warn("slow scheduled job execution",
				zap.String("job_name", name),
				zap.Int64("duration_ms", duration.Milliseconds()))
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

	// Log the scheduled job
	nextRunFields := []zap.Field{
		zap.String("job_name", name),
		zap.String("cron", cronExpr),
	}

	if nextRun, err := scheduledJob.NextRun(); err == nil {
		nextRunFields = append(nextRunFields, zap.String("next_run", nextRun.Format(time.RFC3339)))
	}

	s.logger.Info("job scheduled", nextRunFields...)

	return nil
}

// Stop gracefully shuts down the scheduler and waits for all running jobs
// to complete before returning.
//
// Returns an error if the shutdown process fails.
func (s *Scheduler) Stop() error {
	if err := s.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

// gocronLogAdapter is an adapter to use zap with gocron's logger interface
type gocronLogAdapter struct {
	logger *zap.Logger
}

func (l *gocronLogAdapter) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, zap.Any("args", args))
}

func (l *gocronLogAdapter) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, zap.Any("args", args))
}

func (l *gocronLogAdapter) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, zap.Any("args", args))
}

func (l *gocronLogAdapter) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, zap.Any("args", args))
}
