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

// gormLogger implements GORM's logger.Interface using our logging package.
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
