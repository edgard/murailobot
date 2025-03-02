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
const (
	defaultLogLevel = slog.LevelInfo
	logLevelDebug   = "debug"
	logLevelWarn    = "warn"
	logLevelError   = "error"
	logLevelInfo    = "info"
)

// Supported log output formats.
const (
	logFormatText = "text"
	logFormatJSON = "json"
)

// Error definitions.
var (
	ErrInvalidLogLevel  = errors.New("invalid log level")
	ErrInvalidLogFormat = errors.New("invalid log format")
)

// SetupLogger configures application logging.
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
