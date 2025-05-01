// Package tasks provides interfaces, dependencies, and implementations
// for scheduled background tasks.
package tasks

import (
	"context"
	"fmt"
	"time"
)

// newSQLMaintenanceTask creates the scheduled task function for running database maintenance.
func newSQLMaintenanceTask(deps TaskDeps) ScheduledTaskFunc {
	log := deps.Logger.With("task", "sql_maintenance")

	// This function is the actual task executed by the scheduler.
	// It calls the RunSQLMaintenance method on the database store.
	return func(ctx context.Context) error {
		log.InfoContext(ctx, "Starting scheduled SQL maintenance task...")
		startTime := time.Now()

		// Run the maintenance operation defined in the store
		err := deps.Store.RunSQLMaintenance(ctx)

		duration := time.Since(startTime)

		if err != nil {
			log.ErrorContext(ctx, "SQL maintenance task failed", "error", err, "duration", duration)
			// Wrap the error for the scheduler to potentially handle
			return fmt.Errorf("sql maintenance failed: %w", err)
		}

		log.InfoContext(ctx, "Scheduled SQL maintenance task completed successfully", "duration", duration)
		return nil
	}
}
