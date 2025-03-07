// Package main implements a Telegram bot with AI-powered responses
// and conversation history management.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/telegram"
	"github.com/edgard/murailobot/internal/utils/logging"
)

func main() {
	code := run()
	os.Exit(code)
}

// run initializes components and manages the application lifecycle.
// Returns 0 for success, 1 for error.
func run() int {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)

		return 1
	}

	if err := logging.SetupLogger(cfg); err != nil {
		slog.Error("failed to setup logger", "error", err)

		return 1
	}

	slog.Info("configuration loaded successfully")
	slog.Info("logger initialized", "level", cfg.LogLevel, "format", cfg.LogFormat)

	database, err := db.New()
	if err != nil {
		slog.Error("failed to initialize database", "error", err)

		return 1
	}

	defer func() {
		if err := database.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	aiClient, err := ai.New(cfg, database)
	if err != nil {
		slog.Error("failed to initialize AI client", "error", err)

		return 1
	}

	bot, err := telegram.New(cfg, database, aiClient)
	if err != nil {
		slog.Error("failed to initialize Telegram bot", "error", err)

		return 1
	}

	slog.Info("application initialized successfully",
		"version", version,
		"commit", commit,
		"build_date", date,
		"built_by", builtBy)

	botErr := make(chan error, 1)
	go func() {
		botErr <- bot.Start()
	}()

	var exitCode int
	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)

		if err := bot.Stop(); err != nil {
			slog.Error("failed to stop bot", "error", err)

			exitCode = 1
		}
	case err := <-botErr:
		if err != nil {
			slog.Error("bot stopped with error", "error", err)

			exitCode = 1
		} else {
			slog.Info("bot stopped gracefully")
		}
	}

	slog.Info("application shutdown complete")

	return exitCode
}
