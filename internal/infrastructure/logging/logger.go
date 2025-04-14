// Package logging provides centralized logging capabilities for MurailoBot.
package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/edgard/murailobot/internal/infrastructure/config"
)

// Named creates a named logger that inherits the configuration of the parent.
func Named(logger *zap.Logger, name string) *zap.Logger {
	return logger.Named(name)
}

// ConfigureLogger updates the logger configuration based on application settings.
func ConfigureLogger(logger *zap.Logger, cfg *config.Config) error {
	// Determine log level
	var level zapcore.Level
	switch cfg.LogLevel {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}

	// Build config based on format
	var logConfig zap.Config
	if cfg.LogFormat == "text" {
		logConfig = zap.NewDevelopmentConfig()
	} else {
		logConfig = zap.NewProductionConfig()
	}
	logConfig.Level = zap.NewAtomicLevelAt(level)

	// Build the new logger
	configuredLogger, err := logConfig.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger config: %w", err)
	}

	// Replace the global logger for compatibility with any direct zap.L() uses
	zap.ReplaceGlobals(configuredLogger)

	// Also replace the fields in the existing logger instance
	*logger = *configuredLogger.Named("app")

	logger.Info("logger successfully configured",
		zap.String("level", cfg.LogLevel),
		zap.String("format", cfg.LogFormat))

	return nil
}
