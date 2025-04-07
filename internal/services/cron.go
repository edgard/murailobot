package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/go-co-op/gocron/v2"
)

// CronConfig holds configuration for the scheduler
type CronConfig struct {
	TimeZone string // Time zone for scheduling (defaults to "UTC")
}

// Cron implements the Scheduler interface using gocron
type Cron struct {
	scheduler gocron.Scheduler
	config    CronConfig
	jobs      map[string]gocron.Job
	jobsMu    sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// Convert UUID bytes to hex string
func uuidToString(uuid [16]byte) string {
	return hex.EncodeToString(uuid[:])
}

// NewCron creates a new cron scheduler instance
func NewCron(config CronConfig) (interfaces.Scheduler, error) {
	if config.TimeZone == "" {
		config.TimeZone = "UTC"
	}

	loc, err := time.LoadLocation(config.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", common.ErrInvalidTimeZone, config.TimeZone)
	}

	scheduler, err := gocron.NewScheduler(
		gocron.WithLocation(loc),
		gocron.WithLogger(&gocronLogAdapter{}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create scheduler", common.ErrInitialization)
	}

	return &Cron{
		scheduler: scheduler,
		config:    config,
		jobs:      make(map[string]gocron.Job),
	}, nil
}

// Start begins scheduler operation
func (c *Cron) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.scheduler.Start()
	slog.Info("scheduler started", "timezone", c.config.TimeZone)
	return nil
}

// Stop gracefully shuts down the scheduler
func (c *Cron) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}

	if err := c.scheduler.Shutdown(); err != nil {
		return fmt.Errorf("%w: failed to shutdown scheduler", common.ErrServiceStop)
	}

	slog.Info("scheduler stopped")
	return nil
}

// AddJob adds a new job to the scheduler
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

	c.jobsMu.Lock()
	defer c.jobsMu.Unlock()

	// Check if job already exists
	if _, exists := c.jobs[name]; exists {
		return fmt.Errorf("%w: %s", common.ErrDuplicateJob, name)
	}

	// Create a wrapper that handles context and panic recovery
	wrappedJob := func() {
		// Check context before executing
		select {
		case <-c.ctx.Done():
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
