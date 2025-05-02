package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"

	"github.com/edgard/murailobot/internal/bot/tasks"
	"github.com/edgard/murailobot/internal/config"
)

// Scheduler manages scheduled tasks using gocron v2.
// It handles task registration, execution, and graceful shutdown.
type Scheduler struct {
	scheduler gocron.Scheduler
	logger    *slog.Logger
	cfg       *config.SchedulerConfig
	taskMap   map[string]tasks.ScheduledTaskFunc
	mu        sync.Mutex
	running   bool
}

// NewScheduler creates a new scheduler with the provided logger, configuration, and task map.
// Task map contains task names as keys and their implementation functions as values.
func NewScheduler(logger *slog.Logger, cfg *config.SchedulerConfig, taskMap map[string]tasks.ScheduledTaskFunc) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "scheduler")

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Error("Failed to create gocron scheduler", "error", err)

		panic(fmt.Sprintf("failed to create gocron scheduler: %v", err))
	}

	return &Scheduler{
		scheduler: s,
		logger:    log,
		cfg:       cfg,
		taskMap:   taskMap,
	}
}

// Start initializes and starts all scheduled tasks.
// It returns an error if any task fails to be scheduled.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.logger.Debug("Configuring scheduler jobs...")

	if s.cfg == nil || len(s.cfg.Tasks) == 0 {
		s.logger.Warn("No scheduler tasks configured.")
		s.scheduler.Start()
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

		_, err := s.scheduler.NewJob(
			gocron.CronJob(taskConfig.Schedule, true),
			gocron.NewTask(

				func(ctx context.Context, name string) {
					s.logger.Info("Running scheduled task", "task_name", name)
					startTime := time.Now()

					if taskErr := taskFunc(ctx); taskErr != nil {
						s.logger.Error("Scheduled task failed", "task_name", name, "error", taskErr)
					}
					duration := time.Since(startTime)
					s.logger.Info("Finished scheduled task", "task_name", name, "duration", duration)
				},
				context.Background(),
				taskName,
			),
			gocron.WithName(taskName),
		)
		if err != nil {
			s.logger.Error("Failed to schedule task", "task_name", taskName, "schedule", taskConfig.Schedule, "error", err)
			continue
		}

		s.logger.Info("Scheduled task", "task_name", taskName, "schedule", taskConfig.Schedule)
		scheduledCount++
	}

	s.scheduler.Start()
	s.running = true
	s.logger.Info("Scheduler initialized and started", "tasks_scheduled", scheduledCount)

	return nil
}

// Stop gracefully shuts down the scheduler and waits for all tasks to complete.
// It returns an error if the shutdown process fails or times out.
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		s.logger.Info("Scheduler is not running, nothing to stop.")
		return nil
	}

	s.logger.Debug("Stopping scheduler gracefully (waiting for jobs)...")
	err := s.scheduler.Shutdown()
	if err != nil {
		s.logger.Error("Error during scheduler shutdown", "error", err)
	} else {
		s.logger.Info("Scheduler stopped gracefully.")
	}

	s.running = false
	return err
}
