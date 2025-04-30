package tasks

import (
	"context"
	// "log/slog" // Removed unused import
	// "github.com/go-telegram/bot" // Bot instance is now in TaskDeps
	// "github.com/edgard/murailobot/internal/config" // Config is in TaskDeps
	// "github.com/edgard/murailobot/internal/database" // Store is in TaskDeps
	// "github.com/edgard/murailobot/internal/gemini" // GeminiClient is in TaskDeps
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
	// tasks["profile_update"] = newProfileUpdateTask(deps) // Removed profile update task

	// Add more tasks here by calling their respective factory functions:
	// tasks["daily_report"] = newDailyReportTask(deps)
	// tasks["cleanup_old_data"] = newCleanupTask(deps)

	// Use logger from deps
	deps.Logger.Info("Initialized scheduled tasks", "count", len(tasks))
	return tasks
}

// --- Placeholder Task Factories ---
// Implementations are now in separate files (e.g., sql_maintenance_task.go, profile_update.go)
