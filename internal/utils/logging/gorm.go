package logging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultSlowThreshold = 200 * time.Millisecond
)

// GormLogger implements GORM's logger.Interface using our logging package.
type GormLogger struct {
	SlowThreshold time.Duration
}

// NewGormLogger creates a new GORM logger that uses our logging implementation.
// Interface return is required to satisfy GORM's logger.Interface contract
//
//nolint:ireturn // Intentionally returning interface to implement GORM's logger system
func NewGormLogger() logger.Interface {
	return &GormLogger{
		SlowThreshold: defaultSlowThreshold,
	}
}

// LogMode implements logger.Interface.
// Interface return is required to satisfy GORM's logger.Interface contract
//
//nolint:ireturn // Intentionally returning interface to implement GORM's logger system
func (l *GormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l // We use our own log levels
}

// Info implements logger.Interface.
func (l *GormLogger) Info(_ context.Context, msg string, data ...interface{}) {
	Info(msg, "data", fmt.Sprint(data...))
}

// Warn implements logger.Interface.
func (l *GormLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	Warn(msg, "data", fmt.Sprint(data...))
}

// Error implements logger.Interface.
func (l *GormLogger) Error(_ context.Context, msg string, data ...interface{}) {
	Error(msg, "data", fmt.Sprint(data...))
}

// Trace implements logger.Interface.
func (l *GormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []interface{}{
		"elapsed", elapsed,
		"rows", rows,
		"sql", sql,
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		fields = append(fields, "error", err)
		Error("database query", fields...)

		return
	}

	if elapsed > l.SlowThreshold {
		Warn("slow query", fields...)

		return
	}

	Debug("database query", fields...)
}
