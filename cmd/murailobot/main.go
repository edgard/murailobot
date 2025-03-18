// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edgard/murailobot/internal/app"
)

func main() {
	// Set up a default logger for early initialization and startup errors
	// app.New() will reconfigure this logger with settings from config.yaml
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("starting MurailoBot")
	application, err := app.New()
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)

	go func() {
		if err := application.Start(errCh); err != nil {
			errCh <- err
		}
	}()

	var exitCode int
	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)

		// Create a context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := application.Stop(ctx); err != nil {
			exitCode = 1
		}
	case err := <-errCh:
		slog.Error("application error", "error", err)

		// Create a context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = application.Stop(ctx)

		exitCode = 1
	}

	slog.Debug("exiting", "code", exitCode)
	os.Exit(exitCode)
}
