// Package logging provides structured logging configuration.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/errs"
)

// Setup configures the global logger with the specified configuration.
func Setup(cfg *config.Config) error {
	if cfg == nil {
		return errs.NewValidationError("nil config", nil)
	}

	level := slog.LevelInfo

	switch strings.ToLower(cfg.LogLevel) {
	case logLevelDebug:
		level = slog.LevelDebug
	case logLevelWarn:
		level = slog.LevelWarn
	case logLevelError:
		level = slog.LevelError
	case logLevelInfo:
	default:
		return errs.NewValidationError("invalid log level", nil)
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler

	switch strings.ToLower(cfg.LogFormat) {
	case logFormatText:
		handler = slog.NewTextHandler(os.Stderr, opts)
	case logFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return errs.NewValidationError("invalid log format", nil)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	Info("logger initialized",
		"level", cfg.LogLevel,
		"format", cfg.LogFormat)

	return nil
}

// Debug logs a message at debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs a message at info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs a message at warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs a message at error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
