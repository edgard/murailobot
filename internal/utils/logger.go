package utils

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
)

// Constants for magic values.
const (
	defaultLogLevel = slog.LevelInfo
	logLevelDebug   = "debug"
	logLevelWarn    = "warn"
	logLevelError   = "error"
	logLevelInfo    = "info"
	logFormatText   = "text"
	logFormatJSON   = "json"
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
		return fmt.Errorf("invalid log level: %s", cfg.LogLevel)
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler

	switch strings.ToLower(cfg.LogFormat) {
	case logFormatText:
		handler = slog.NewTextHandler(os.Stderr, opts)
	case logFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("invalid log format: %s", cfg.LogFormat)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
