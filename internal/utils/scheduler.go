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
	s, err := gocron.NewScheduler(
		gocron.WithLocation(time.UTC),
		gocron.WithLogger(&gocronLogAdapter{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	s.Start()

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
	if name == "" {
		return errors.New("empty job name")
	}

	if cronExpr == "" {
		return errors.New("empty cron expression")
	}

	if job == nil {
		return errors.New("nil job function")
	}

	_, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(job),
		gocron.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}

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
