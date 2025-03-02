// Package main implements a Telegram bot that integrates with OpenAI's API.
// It provides a conversational interface through Telegram, processing messages
// using OpenAI's language models and maintaining conversation history in a local database.
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

// Build information injected at compile time through linker flags.
// These values provide version tracking and build provenance information
// that can be logged during runtime.
var (
	version = "dev"     // Semantic version of the build
	commit  = "none"    // Git commit hash
	date    = "unknown" // Build timestamp
	builtBy = "unknown" // Builder identifier
)

func main() {
	code := run()
	os.Exit(code)
}

// run initializes and starts the application components in the following order:
//  1. Sets up structured logging
//  2. Loads configuration from file/environment
//  3. Initializes the database connection
//  4. Creates OpenAI client
//  5. Creates and starts Telegram bot
//
// The function handles graceful shutdown on SIGINT/SIGTERM signals.
// It returns an exit code (0 for success, 1 for error) that can be used
// by the main function to terminate the program.
func run() int {
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

	openAIClient, err := openai.New(cfg, database)
	if err != nil {
		slog.Error("failed to initialize OpenAI client", "error", err)

		return 1
	}

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	if err := bot.Start(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("bot stopped with error", "error", err)

			return 1
		}

		slog.Info("bot stopped due to context cancellation")
	}

	slog.Info("application shutdown complete")

	return 0
}
