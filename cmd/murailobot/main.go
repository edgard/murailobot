// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/di"
)

// configureLogger configures an existing logger based on the provided configuration
func configureLogger(cfg *config.Config) error {
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

	var config zap.Config
	if cfg.LogFormat == "text" {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}
	config.Level = zap.NewAtomicLevelAt(level)

	logger, err := config.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger config: %w", err)
	}

	zap.ReplaceGlobals(logger)
	return nil
}

func main() {
	// Create initial logger with minimal config
	logger := zap.NewExample()
	defer logger.Sync() //nolint: errcheck

	app := fx.New(
		// Provide initial logger for fx startup
		fx.Supply(logger),
		// Configure fx logging
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger.Named("fx")}
		}),
		// Include all application modules
		di.RootModule,
		// Configure the logger based on app config
		fx.Invoke(func(lc fx.Lifecycle, cfg *config.Config) error {
			if err := configureLogger(cfg); err != nil {
				return err
			}
			return nil
		}),
	)

	app.Run()
}
