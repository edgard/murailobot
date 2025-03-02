package utils

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/edgard/murailobot/internal/config"
)

// SetupLogger configures application logging.
func SetupLogger(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}

	level := slog.LevelInfo // default
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		// Already set to default
	default:
		return fmt.Errorf("invalid log level: %s", cfg.LogLevel)
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler

	switch strings.ToLower(cfg.LogFormat) {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("invalid log format: %s", cfg.LogFormat)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
