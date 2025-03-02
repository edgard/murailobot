// Package utils provides utility functions for the application,
// including logging configuration and text sanitization.
package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
)

// Default log level used when none is specified.
// These constants define the available logging levels
// in order of increasing severity.
const (
	defaultLogLevel = slog.LevelInfo // Default logging level
	logLevelDebug   = "debug"        // Debug level for detailed troubleshooting
	logLevelWarn    = "warn"         // Warning level for potential issues
	logLevelError   = "error"        // Error level for actual failures
	logLevelInfo    = "info"         // Info level for general operational events
)

// Supported log output formats define the available
// formatting options for log output.
const (
	logFormatText = "text" // Human-readable text format
	logFormatJSON = "json" // Machine-readable JSON format
)

// Error definitions for logging configuration.
var (
	ErrInvalidLogLevel  = errors.New("invalid log level")  // Unsupported log level
	ErrInvalidLogFormat = errors.New("invalid log format") // Unsupported log format
)

// SetupLogger configures application logging based on the provided configuration.
// It sets up the global logger with the specified:
//   - Log level (debug, info, warn, error)
//   - Output format (text or JSON)
//   - Output destination (stderr)
//
// If no configuration is provided, the function returns without error,
// leaving the default logging configuration in place.
//
// Example usage:
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

	level := defaultLogLevel

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
