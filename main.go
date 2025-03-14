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
	"github.com/edgard/murailobot/internal/errs"
	"github.com/edgard/murailobot/internal/logging"
	"github.com/edgard/murailobot/internal/scheduler"
	"github.com/edgard/murailobot/internal/telegram"
)

// Build information variables are set during compilation via ldflags.
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

// run initializes components and manages the application lifecycle.
// Returns 0 for success, 1 for error.
func run() int {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Initialize components in order of dependency
	cfg, err := config.LoadConfig()
	if err != nil {
		logging.Error("initialization failed", "component", "config", "error", err)

		return 1
	}

	if err := logging.Setup(cfg); err != nil {
		// Use basic error logging since logger isn't set up yet
		logging.Error("initialization failed", "component", "logger", "error", err)

		return 1
	}

	logging.Info("configuration loaded successfully")

	s, err := scheduler.New()
	if err != nil {
		logging.Error("initialization failed", "component", "scheduler", "error", err)

		return 1
	}

	// Ensure scheduler is cleaned up on exit
	defer func() {
		if err := s.Stop(); err != nil {
			logging.Error("shutdown failed", "component", "scheduler", "error", err)
		}
	}()

	database, err := db.New()
	if err != nil {
		logging.Error("initialization failed", "component", "database", "error", err)

		return 1
	}

	defer func() {
		if err := database.Close(); err != nil {
			logging.Error("shutdown failed", "component", "database", "error", err)
		}
	}()

	aiClient, err := ai.New(cfg, database)
	if err != nil {
		logging.Error("initialization failed", "component", "ai_client", "error", err)

		return 1
	}

	bot, err := telegram.New(cfg, database, aiClient, s)
	if err != nil {
		logging.Error("initialization failed", "component", "telegram_bot", "error", err)

		return 1
	}

	logging.Info("application initialized successfully",
		"version", version,
		"commit", commit,
		"build_date", date,
		"built_by", builtBy)

	// Start bot in a goroutine and handle shutdown
	botErr := make(chan error, 1)

	go func() {
		if err := bot.Start(); err != nil {
			// Wrap bot errors for better context
			if _, ok := err.(*errs.Error); !ok {
				err = errs.NewAPIError("bot runtime error", err)
			}
			botErr <- err
		}
	}()

	var exitCode int
	select {
	case sig := <-quit:
		logging.Info("received shutdown signal", "signal", sig)

		if err := bot.Stop(); err != nil {
			logging.Error("shutdown failed",
				"component", "telegram_bot",
				"error", err)

			exitCode = 1
		}
	case err := <-botErr:
		if err != nil {
			logging.Error("bot stopped with error",
				"error", err,
				"error_type", errs.Code(err))

			exitCode = 1
		} else {
			logging.Info("bot stopped gracefully")
		}
	}

	logging.Info("application shutdown complete", "exit_code", exitCode)

	return exitCode
}
