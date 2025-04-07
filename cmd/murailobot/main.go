package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edgard/murailobot/internal/app"
	"github.com/edgard/murailobot/internal/common"
)

func main() {
	// Set up initial logger for startup
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("starting MurailoBot")

	// Load configuration
	config, err := common.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create application instance
	application, err := app.New(app.LoadFromConfig(config))
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	// Create error channel for runtime errors
	errCh := make(chan error, 1)

	// Start application
	go func() {
		if err := application.Start(errCh); err != nil {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	var exitCode int
	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)

		// Create context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := application.Stop(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
			exitCode = 1
		}

	case err := <-errCh:
		slog.Error("application error", "error", err)

		// Create context with shorter timeout for error shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := application.Stop(ctx); err != nil {
			slog.Error("emergency shutdown error", "error", err)
		}
		exitCode = 1
	}

	os.Exit(exitCode)
}
