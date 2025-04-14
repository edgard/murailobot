// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/fx"

	"github.com/edgard/murailobot/internal/domain/service"
	fxmodules "github.com/edgard/murailobot/internal/fx"
)

func main() {
	// Set up a default logger for early initialization and startup errors
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Application shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Error channel for application runtime errors
	errCh := make(chan error, 1)

	slog.Info("starting MurailoBot")

	// Create and run the application with fx
	app := fx.New(
		// Include all application modules from our fx package
		fxmodules.SimpleRootModule,

		// Invoke application start
		fx.Invoke(runApp(errCh)),
	)

	// Start the application
	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		slog.Error("failed to start application", "error", err)
		os.Exit(1)
	}

	// Wait for application to finish or for shutdown signal
	var exitCode int
	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)

		// Create a context with timeout for shutdown
		stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
		defer cancel()

		if err := app.Stop(stopCtx); err != nil {
			slog.Error("error stopping application", "error", err)
			exitCode = 1
		}
	case err := <-errCh:
		slog.Error("application error", "error", err)

		// Create a context with timeout for shutdown
		stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
		defer cancel()

		_ = app.Stop(stopCtx)
		exitCode = 1
	}

	slog.Info("application shutdown complete")
	os.Exit(exitCode)
}

// runApp initializes and runs the application
func runApp(errCh chan<- error) func(lc fx.Lifecycle, chatService *service.ChatService) {
	return func(lc fx.Lifecycle, chatService *service.ChatService) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				slog.Info("starting MurailoBot service")
				go func() {
					if err := chatService.Start(errCh); err != nil {
						errCh <- err
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				slog.Info("stopping MurailoBot service")
				return chatService.Stop()
			},
		})
	}
}
