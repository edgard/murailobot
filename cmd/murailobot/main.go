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

	// Create root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Start application with root context
	go func() {
		if err := application.Start(ctx, errCh); err != nil {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	var exitCode int
	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)

		// Cancel root context
		cancel()

		// Create context with timeout for graceful shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := application.Stop(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
			exitCode = 1
		}

	case err := <-errCh:
		slog.Error("application error", "error", err)

		// Cancel root context
		cancel()

		// Create context with shorter timeout for error shutdown
		emergencyCtx, emergencyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer emergencyCancel()

		if err := application.Stop(emergencyCtx); err != nil {
			slog.Error("emergency shutdown error", "error", err)
		}
		exitCode = 1
	}

	os.Exit(exitCode)
}
