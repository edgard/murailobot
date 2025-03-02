package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/telegram"
	"github.com/edgard/murailobot/internal/utils"
)

// Add build information variables.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	code := run()
	os.Exit(code)
}

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
