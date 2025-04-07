package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/go-co-op/gocron/v2"
)

// Cron implements the Scheduler interface using gocron
type Cron struct {
	scheduler gocron.Scheduler
	jobs      map[string]gocron.Job
	stopCh    chan struct{}
	timezone  string
}

// NewCron creates a new cron scheduler instance
func NewCron() (interfaces.Scheduler, error) {
	return &Cron{
		jobs:   make(map[string]gocron.Job),
		stopCh: make(chan struct{}),
	}, nil
}

// Configure sets up scheduler with given timezone
func (c *Cron) Configure(timezone string) error {
	if timezone == "" {
		timezone = "UTC"
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrInvalidTimeZone, timezone)
	}

	scheduler, err := gocron.NewScheduler(
		gocron.WithLocation(loc),
		gocron.WithLogger(&gocronLogAdapter{}),
	)
	if err != nil {
		return fmt.Errorf("%w: failed to create scheduler", common.ErrInitialization)
	}

	c.scheduler = scheduler
	c.timezone = timezone
	return nil
}

// AddJob adds a new scheduled job
func (c *Cron) AddJob(name, cronExpr string, job func()) error {
	if name == "" {
		return common.ErrEmptyJobName
	}

	if cronExpr == "" {
		return common.ErrEmptyCronExpression
	}

	if job == nil {
		return common.ErrNilJobFunction
	}

	// Check if job already exists
	if _, exists := c.jobs[name]; exists {
		return fmt.Errorf("%w: %s", common.ErrDuplicateJob, name)
	}

	// Create a wrapper that handles panic recovery
	wrappedJob := func() {
		// Check if scheduler is stopping
		select {
		case <-c.stopCh:
			return
		default:
			// Continue with execution
		}

		// Run the job with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("job panic recovered",
						"job_name", name,
						"error", r)
				}
			}()

			job()
		}()
	}

	// Schedule the job
	scheduledJob, err := c.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(wrappedJob),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return fmt.Errorf("%w: %s", common.ErrJobSchedule, err)
	}

	// Store the job reference
	c.jobs[name] = scheduledJob

	// Log scheduling details
	logAttrs := []interface{}{
		"job_name", name,
		"job_id", uuidToString(scheduledJob.ID()),
		"cron", cronExpr,
	}

	if nextRun, err := scheduledJob.NextRun(); err == nil {
		logAttrs = append(logAttrs, "next_run", nextRun.Format(time.RFC3339))
	}

	slog.Info("job scheduled", logAttrs...)

	return nil
}

// Start begins scheduler operation
func (c *Cron) Start(ctx context.Context) error {
	c.scheduler.Start()
	slog.Info("scheduler started", "timezone", c.timezone)

	// Monitor context for cancellation
	go func() {
		select {
		case <-ctx.Done():
			c.stopCh <- struct{}{}
		case <-c.stopCh:
		}
	}()

	return nil
}

// Stop gracefully shuts down the scheduler
func (c *Cron) Stop() error {
	select {
	case c.stopCh <- struct{}{}:
	default:
	}

	if err := c.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("%w: failed to shutdown scheduler", common.ErrServiceStop)
	}

	slog.Info("scheduler stopped")
	return nil
}

// Convert UUID bytes to hex string
func uuidToString(uuid [16]byte) string {
	return hex.EncodeToString(uuid[:])
}

// gocronLogAdapter adapts gocron logging to slog
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

// toSlogArgs converts variadic args to slog-compatible args
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
