package tasks

import (
	"context"
	"fmt"
	"time"
)

func newSQLMaintenanceTask(deps TaskDeps) ScheduledTaskFunc {
	log := deps.Logger.With("task", "sql_maintenance")

	return func(ctx context.Context) error {
		log.InfoContext(ctx, "Starting scheduled SQL maintenance task...")
		startTime := time.Now()

		err := deps.Store.RunSQLMaintenance(ctx)

		duration := time.Since(startTime)

		if err != nil {
			log.ErrorContext(ctx, "SQL maintenance task failed", "error", err, "duration", duration)

			return fmt.Errorf("sql maintenance failed: %w", err)
		}

		log.InfoContext(ctx, "Scheduled SQL maintenance task completed successfully", "duration", duration)
		return nil
	}
}
