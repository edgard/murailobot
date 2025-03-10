// Package main implements a Telegram bot with AI-powered responses
// and conversation history management.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/scheduler"
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

	cfg, err := config.LoadConfig()
	if err != nil {
		logging.Error("failed to load configuration", "error", err)

		return 1
	}

	if err := logging.Setup(cfg); err != nil {
		logging.Error("failed to setup logger", "error", err)

		return 1
	}

	logging.Info("configuration loaded successfully")

	s, err := scheduler.New()
	if err != nil {
		logging.Error("failed to create scheduler", "error", err)

		return 1
	}

	// Ensure scheduler is cleaned up on exit
	defer func() {
		if err := s.Stop(); err != nil {
			logging.Error("failed to close scheduler", "error", err)
		}
	}()

	database, err := db.New()
	if err != nil {
		logging.Error("failed to initialize database", "error", err)

		return 1
	}

	defer func() {
		if err := database.Close(); err != nil {
			logging.Error("failed to close database", "error", err)
		}
	}()

	aiClient, err := ai.New(cfg, database)
	if err != nil {
		logging.Error("failed to initialize AI client", "error", err)

		return 1
	}

	bot, err := telegram.New(cfg, database, aiClient, s)
	if err != nil {
		logging.Error("failed to initialize Telegram bot", "error", err)

		return 1
	}

	logging.Info("application initialized successfully",
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
		logging.Info("received shutdown signal", "signal", sig)

		if err := bot.Stop(); err != nil {
			logging.Error("failed to stop bot", "error", err)

			exitCode = 1
		}
	case err := <-botErr:
		if err != nil {
			logging.Error("bot stopped with error", "error", err)

			exitCode = 1
		} else {
			logging.Info("bot stopped gracefully")
		}
	}

	logging.Info("application shutdown complete")

	return exitCode
}
