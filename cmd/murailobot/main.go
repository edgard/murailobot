// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/di"
)

func main() {
	app := fx.New(
		// Configure fx logging
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger.Named("fx")}
		}),
		// Include all application modules
		di.RootModule,
	)

	app.Run()
}
