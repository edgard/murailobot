// Package main implements a Telegram bot that integrates with OpenAI's API,
// providing a conversational interface through Telegram and maintaining
// conversation history in a local database.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
	"github.com/edgard/murailobot/internal/telegram"
	"github.com/edgard/murailobot/internal/utils"
)

func main() {
	code := run()
	os.Exit(code)
}

// run initializes and starts the application components, handling graceful
// shutdown on SIGINT/SIGTERM signals. It returns an exit code (0 for success,
// 1 for error) for the main function to use when terminating the program.
func run() int {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)

		return 1
	}

	if err := utils.SetupLogger(cfg); err != nil {
		slog.Error("failed to setup logger", "error", err)

		return 1
	}

	slog.Info("configuration loaded successfully")
	slog.Info("logger initialized", "level", cfg.LogLevel, "format", cfg.LogFormat)

	// Initialize database
	database, err := db.New(nil) // Use db package defaults
	if err != nil {
		slog.Error("failed to initialize database", "error", err)

		return 1
	}

	defer func() {
		if err := database.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	// Initialize OpenAI client
	openAIClient, err := openai.New(cfg, database)
	if err != nil {
		slog.Error("failed to initialize OpenAI client", "error", err)

		return 1
	}

	// Initialize Telegram bot
	bot, err := telegram.New(cfg, database, openAIClient)
	if err != nil {
		slog.Error("failed to initialize Telegram bot", "error", err)

		return 1
	}

	slog.Info("application initialized successfully",
		"version", version,
		"commit", commit,
		"build_date", date,
		"built_by", builtBy)

	// Handle shutdown signals in a separate goroutine
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		slog.Info("received shutdown signal", "signal", sig)

		// Stop the bot
		if err := bot.Stop(); err != nil {
			slog.Error("failed to stop bot", "error", err)
		}

		// Cancel the root context to signal shutdown to all components
		rootCancel()
	}()

	if err := bot.Start(rootCtx); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("bot stopped with error", "error", err)

			return 1
		}

		slog.Info("bot stopped due to context cancellation")
	}

	slog.Info("application shutdown complete")

	return 0
}
