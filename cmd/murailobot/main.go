// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/di"
)

func main() {
	// Create a base zap logger for startup
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	logger.Info("starting MurailoBot")

	// Create and run the application with fx
	app := fx.New(
		// Provide the base logger
		fx.Supply(logger),
		// Configure FX to use our zap logger
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),
		// Include all application modules from our di package
		di.RootModule,
	)
	app.Run()
}
