// Package utils provides utility functions for the application.
package utils

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
)

// SetupLogger configures application logging based on the provided configuration.
// Sets up the global logger with the specified level, format, and output destination.
// Returns without error if no configuration is provided, leaving default logging in place.
//
// Example:
//
//	cfg := &config.Config{
//	    LogLevel:  "debug",
//	    LogFormat: "json",
//	}
//	if err := SetupLogger(cfg); err != nil {
//	    log.Fatal(err)
//	}
func SetupLogger(cfg *config.Config) error {
	if cfg == nil {
		return nil
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
		// Already set to default
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogLevel, cfg.LogLevel)
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler

	switch strings.ToLower(cfg.LogFormat) {
	case logFormatText:
		handler = slog.NewTextHandler(os.Stderr, opts)
	case logFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogFormat, cfg.LogFormat)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
