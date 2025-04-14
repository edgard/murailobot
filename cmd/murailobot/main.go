// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"log/slog"
	"os"

	"go.uber.org/fx"

	"github.com/edgard/murailobot/internal/di"
)

func main() {
	// Set up a default logger for early initialization and startup errors
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	slog.Info("starting MurailoBot")

	// Create and run the application with fx
	fx.New(
		// Include all application modules from our di package
		di.RootModule,
	).Run()
}
