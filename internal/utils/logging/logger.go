// Package logging provides structured logging configuration.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
)

// SetupLogger configures the global logger with the specified level and format.
func SetupLogger(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}

	level := slog.LevelInfo

	switch strings.ToLower(cfg.LogLevel) {
	case LogLevelDebug:
		level = slog.LevelDebug
	case LogLevelWarn:
		level = slog.LevelWarn
	case LogLevelError:
		level = slog.LevelError
	case LogLevelInfo:
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogLevel, cfg.LogLevel)
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler

	switch strings.ToLower(cfg.LogFormat) {
	case LogFormatText:
		handler = slog.NewTextHandler(os.Stderr, opts)
	case LogFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogFormat, cfg.LogFormat)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
