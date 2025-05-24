package tasks

import (
	"context"
)

// ScheduledTaskFunc is the function type for scheduled tasks.
// Each task receives a context and returns an error if the task execution fails.
type ScheduledTaskFunc func(ctx context.Context) error

// RegisterAllTasks initializes and returns a map of all available scheduled tasks.
// The map keys are task names and values are the task implementation functions.
func RegisterAllTasks(deps TaskDeps) map[string]ScheduledTaskFunc {
	tasks := make(map[string]ScheduledTaskFunc)

	tasks["sql_maintenance"] = newSQLMaintenanceTask(deps)
	tasks["profile_analysis"] = newProfileAnalysisTask(deps)

	deps.Logger.Info("Initialized scheduled tasks", "count", len(tasks))
	return tasks
}
