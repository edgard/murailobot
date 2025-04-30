package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time" // gocron uses time.Duration

	"github.com/go-co-op/gocron/v2" // Replaced cron library

	"github.com/edgard/murailobot/internal/bot/tasks"
	"github.com/edgard/murailobot/internal/config"
)

// Scheduler manages scheduled tasks using the gocron library.
type Scheduler struct {
	scheduler gocron.Scheduler // Changed from cron.Cron
	logger    *slog.Logger
	cfg       *config.SchedulerConfig
	taskMap   map[string]tasks.ScheduledTaskFunc // Map of registered task functions
	mu        sync.Mutex                         // To protect access during start/stop
	running   bool
}

// NewScheduler creates a new scheduler instance using gocron.
func NewScheduler(logger *slog.Logger, cfg *config.SchedulerConfig, taskMap map[string]tasks.ScheduledTaskFunc) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "scheduler")

	// Create a new gocron scheduler
	s, err := gocron.NewScheduler()
	if err != nil {
		// This error typically occurs only if time.LoadLocation fails, which is rare.
		log.Error("Failed to create gocron scheduler", "error", err)
		// Handle error appropriately, maybe panic or return an error
		panic(fmt.Sprintf("failed to create gocron scheduler: %v", err))
	}

	// gocron v2 doesn't have built-in structured logging integration like slog yet.
	// We can log before/after job execution within the task wrapper if needed.

	return &Scheduler{
		scheduler: s,
		logger:    log,
		cfg:       cfg,
		taskMap:   taskMap,
	}
}

// Start schedules and starts all enabled tasks based on the configuration.
// Note: gocron starts jobs immediately upon addition unless configured otherwise.
// The StartAsync method starts the scheduler's internal ticking.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.logger.Debug("Configuring scheduler jobs...")

	if s.cfg == nil || len(s.cfg.Tasks) == 0 {
		s.logger.Warn("No scheduler tasks configured.")
		s.scheduler.Start() // Start the scheduler even if no jobs yet
		s.running = true
		return nil
	}

	scheduledCount := 0
	for taskName, taskConfig := range s.cfg.Tasks {
		if !taskConfig.Enabled {
			s.logger.Info("Skipping disabled task", "task_name", taskName)
			continue
		}

		taskFunc, exists := s.taskMap[taskName]
		if !exists {
			s.logger.Warn("Scheduled task configured but not found in registry, skipping", "task_name", taskName)
			continue
		}

		if taskConfig.Schedule == "" {
			s.logger.Warn("Scheduled task enabled but has empty schedule, skipping", "task_name", taskName)
			continue
		}

		// Define the job using gocron's fluent API
		_, err := s.scheduler.NewJob(
			gocron.CronJob(taskConfig.Schedule, true), // true = use seconds field if present
			gocron.NewTask(
				func(ctx context.Context, name string) { // Wrap the original task func
					s.logger.Info("Running scheduled task", "task_name", name)
					startTime := time.Now()
					// Pass the context provided by gocron to the task and capture error
					if taskErr := taskFunc(ctx); taskErr != nil {
						s.logger.Error("Scheduled task failed", "task_name", name, "error", taskErr)
					}
					duration := time.Since(startTime)
					s.logger.Info("Finished scheduled task", "task_name", name, "duration", duration)
				},
				context.Background(), // Base context for the task wrapper
				taskName,             // Pass task name to the wrapper
			),
			gocron.WithName(taskName), // Set a name for the job for logging/management
			// gocron.WithSingletonMode(gocron.LimitModeReschedule), // Prevent job overrun
		)
		if err != nil {
			s.logger.Error("Failed to schedule task", "task_name", taskName, "schedule", taskConfig.Schedule, "error", err)
			continue // Continue scheduling other tasks
		}

		s.logger.Info("Scheduled task", "task_name", taskName, "schedule", taskConfig.Schedule)
		scheduledCount++
	}

	// Start the scheduler's async loop
	s.scheduler.Start()
	s.running = true
	s.logger.Info("Scheduler initialized and started", "tasks_scheduled", scheduledCount)

	return nil
}

// Stop gracefully stops the scheduler, waiting for running jobs to complete.
// gocron's Shutdown waits for jobs.
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		s.logger.Info("Scheduler is not running, nothing to stop.")
		return nil
	}

	s.logger.Debug("Stopping scheduler gracefully (waiting for jobs)...")
	err := s.scheduler.Shutdown() // Shutdown waits for running jobs
	if err != nil {
		s.logger.Error("Error during scheduler shutdown", "error", err)
		// Fallthrough to mark as not running anyway
	} else {
		s.logger.Info("Scheduler stopped gracefully.")
	}

	s.running = false
	return err // Return shutdown error if any
}

// --- Old cron logger adapters removed ---
