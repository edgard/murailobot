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
	"github.com/edgard/murailobot/internal/services"
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

	// Create and configure database service
	db, err := services.NewSQL()
	if err != nil {
		slog.Error("failed to create database service", "error", err)
		os.Exit(1)
	}
	if err := db.Configure(config.DBPath); err != nil {
		slog.Error("failed to configure database", "error", err)
		os.Exit(1)
	}

	// Create and configure scheduler service
	scheduler, err := services.NewCron()
	if err != nil {
		slog.Error("failed to create scheduler service", "error", err)
		os.Exit(1)
	}
	if err := scheduler.Configure("UTC"); err != nil {
		slog.Error("failed to configure scheduler", "error", err)
		os.Exit(1)
	}

	// Create and configure Telegram bot service
	bot, err := services.NewTelegram()
	if err != nil {
		slog.Error("failed to create bot service", "error", err)
		os.Exit(1)
	}
	if err := bot.Configure(config.BotToken, config.BotAdminID, config.AIMaxContextTokens/2); err != nil {
		slog.Error("failed to configure bot", "error", err)
		os.Exit(1)
	}

	// Create and configure AI service
	ai, err := services.NewOpenAI()
	if err != nil {
		slog.Error("failed to create AI service", "error", err)
		os.Exit(1)
	}
	if err := ai.Configure(
		config.AIToken,
		config.AIBaseURL,
		config.AIModel,
		config.AIMaxContextTokens,
		config.AITemperature,
		config.AITimeout,
		config.AIInstruction,
		config.AIProfileInstruction,
	); err != nil {
		slog.Error("failed to configure AI service", "error", err)
		os.Exit(1)
	}

	// Create and initialize application
	application, err := app.New(config, db, ai, bot, scheduler)
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
