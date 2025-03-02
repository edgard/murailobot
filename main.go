package main

import (
	"context"
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

// Add build information variables
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	if err := utils.SetupLogger(cfg); err != nil {
		slog.Error("failed to setup logger", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded successfully")
	slog.Info("logger initialized", "level", cfg.LogLevel, "format", cfg.LogFormat)

	database, err := db.New(nil) // Use db package defaults
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	aiClient, err := ai.New(cfg, database)
	if err != nil {
		slog.Error("failed to initialize AI client", "error", err)
		os.Exit(1)
	}

	bot, err := telegram.New(cfg, database, aiClient)
	if err != nil {
		slog.Error("failed to initialize Telegram bot", "error", err)
		os.Exit(1)
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
		if err != context.Canceled {
			slog.Error("bot stopped with error", "error", err)
			os.Exit(1)
		} else {
			slog.Info("bot stopped due to context cancellation")
		}
	}

	slog.Info("application shutdown complete")
}
