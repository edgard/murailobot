package logging

import (
	"context"
	"errors"
	"fmt"
	"time"

	errs "github.com/edgard/murailobot/internal/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultSlowThreshold = 200 * time.Millisecond
)

// gormLogger implements GORM's logger.Interface using our structured logging package.
type gormLogger struct {
	SlowThreshold time.Duration
}

// NewGormLogger returns a new logger that implements GORM's logger.Interface.
//
//nolint:ireturn // Interface return is required by GORM's API contract
func NewGormLogger() logger.Interface {
	return &gormLogger{
		SlowThreshold: defaultSlowThreshold,
	}
}

// LogMode implements logger.Interface.
//
//nolint:ireturn // Interface return is required by GORM's API contract
func (l *gormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l // We use our own log levels
}

// Info implements logger.Interface.
func (l *gormLogger) Info(_ context.Context, msg string, data ...interface{}) {
	Info(msg, "data", fmt.Sprint(data...))
}

// Warn implements logger.Interface.
func (l *gormLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	Warn(msg, "data", fmt.Sprint(data...))
}

// Error implements logger.Interface.
func (l *gormLogger) Error(_ context.Context, msg string, data ...interface{}) {
	Error(msg, "data", fmt.Sprint(data...))
}

// Trace implements logger.Interface.
func (l *gormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []interface{}{
		"elapsed", elapsed,
		"rows", rows,
		"sql", sql,
	}

	if err != nil {
		// Categorize database errors
		var dbErr error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// Record not found is not an error condition that needs logging
			Debug("database query - no records found", fields...)

			return
		case errors.Is(err, gorm.ErrInvalidTransaction):
			dbErr = errs.NewDatabaseError("invalid transaction", err)
		case errors.Is(err, gorm.ErrNotImplemented):
			dbErr = errs.NewDatabaseError("operation not implemented", err)
		case errors.Is(err, gorm.ErrMissingWhereClause):
			dbErr = errs.NewValidationError("missing where clause", err)
		case errors.Is(err, gorm.ErrUnsupportedRelation):
			dbErr = errs.NewValidationError("unsupported relation", err)
		case errors.Is(err, gorm.ErrPrimaryKeyRequired):
			dbErr = errs.NewValidationError("primary key required", err)
		case errors.Is(err, gorm.ErrModelValueRequired):
			dbErr = errs.NewValidationError("model value required", err)
		case errors.Is(err, gorm.ErrInvalidData):
			dbErr = errs.NewValidationError("invalid data", err)
		case errors.Is(err, gorm.ErrUnsupportedDriver):
			dbErr = errs.NewConfigError("unsupported database driver", err)
		case errors.Is(err, gorm.ErrRegistered):
			dbErr = errs.NewConfigError("duplicate registration", err)
		default:
			dbErr = errs.NewDatabaseError("database error", err)
		}

		fields = append(fields, "error", dbErr)
		Error("database query failed", fields...)

		return
	}

	if elapsed > l.SlowThreshold {
		Warn("slow query detected",
			append(fields,
				"threshold", l.SlowThreshold,
				"exceeded_by", elapsed-l.SlowThreshold,
			)...,
		)

		return
	}

	Debug("database query completed", fields...)
}
