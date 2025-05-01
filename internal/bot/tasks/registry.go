// Package tasks provides interfaces, dependencies, and registration logic
// for scheduled background tasks.
package tasks

import (
	"context"
)

// ScheduledTaskFunc defines the standard signature for all scheduled tasks.
// The context provided by the scheduler should be respected for cancellation.
// Update: Return an error to allow scheduler to handle failures.
type ScheduledTaskFunc func(ctx context.Context) error

// TaskDeps struct is now defined in deps.go

// RegisterAllTasks initializes and returns a map of all registered scheduled tasks.
// It calls the factory function for each task (e.g., newSQLMaintenanceTask).
// The keys of the map are identifiers for the tasks (e.g., "sql_maintenance"),
// used for configuration lookup and potentially logging.
func RegisterAllTasks(deps TaskDeps) map[string]ScheduledTaskFunc {
	tasks := make(map[string]ScheduledTaskFunc)

	// --- Register Tasks ---
	// The key (e.g., "sql_maintenance") should match the key used in the config.yaml scheduler section.
	tasks["sql_maintenance"] = newSQLMaintenanceTask(deps) // Refers to function in sql_maintenance_task.go

	// Add more tasks here by calling their respective factory functions:
	// tasks["daily_report"] = newDailyReportTask(deps)
	// tasks["cleanup_old_data"] = newCleanupTask(deps)

	deps.Logger.Info("Initialized scheduled tasks", "count", len(tasks))
	return tasks
}

// --- Placeholder Task Factories ---
// Implementations are now in separate files (e.g., sql_maintenance_task.go)
