// Package main contains the entrypoint for the Telegram bot application.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbot "github.com/go-telegram/bot"

	"github.com/edgard/murailobot/internal/bot"
	"github.com/edgard/murailobot/internal/bot/handlers"
	"github.com/edgard/murailobot/internal/bot/tasks"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
	"github.com/edgard/murailobot/internal/gemini"
	"github.com/edgard/murailobot/internal/logger"
	"github.com/edgard/murailobot/internal/telegram"

	_ "modernc.org/sqlite"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	exitCode := run(ctx)
	stop() // Ensure context cancellation is signaled before exit
	os.Exit(exitCode)
}

// run initializes and starts all application components (config, logger, db, ai client, bot, scheduler),
// handles graceful shutdown, and returns an exit code (0 for success, 1 for failure).
func run(ctx context.Context) int {
	configPath := flag.String("config", "./config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "path", *configPath, "error", err)
		return 1
	}

	log := logger.NewLogger(cfg.Logger.Level, cfg.Logger.JSON)
	slog.SetDefault(log)
	log.Info("Logger initialized", "level", cfg.Logger.Level, "json", cfg.Logger.JSON)

	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		log.Error("Failed to connect to database", "path", cfg.Database.Path, "error", err)
		return 1
	}
	defer database.CloseDB(db) // Ensure DB is closed on function exit
	store := database.NewStore(db, log)

	gemClient, err := gemini.NewClient(ctx, cfg.Gemini, log)
	if err != nil {
		log.Error("Failed to initialize Gemini client", "error", err)
		return 1
	}
	// Note: Gemini client does not have an explicit Close method in the SDK used.

	hDeps := handlers.HandlerDeps{
		Logger:       log,
		Store:        store,
		GeminiClient: gemClient,
		Config:       cfg,
	}
	tDeps := tasks.TaskDeps{
		Logger:       log,
		Store:        store,
		GeminiClient: gemClient,
		Config:       cfg,
	}

	botOpts := []tgbot.Option{
		tgbot.WithMiddlewares(logger.Middleware(log)),
		tgbot.WithDefaultHandler(handlers.NewMentionHandler(hDeps)),
	}
	tg, err := telegram.NewTelegramBot(cfg.Telegram.Token, log, botOpts...)
	if err != nil {
		log.Error("Failed to create Telegram bot", "error", err)
		return 1
	}

	// Retrieve bot info and store it in the config for runtime use
	cfg.Telegram.BotInfo, err = tg.GetMe(ctx)
	if err != nil {
		log.Error("Failed to get bot info", "error", err)
		return 1
	}
	log.Info("Retrieved bot info", "bot_id", cfg.Telegram.BotInfo.ID, "bot_username", cfg.Telegram.BotInfo.Username)

	cmdHandlers := handlers.RegisterAllCommands(hDeps)
	if err := telegram.RegisterHandlers(tg, log, cmdHandlers); err != nil {
		log.Error("Failed to register Telegram handlers", "error", err)
		return 1
	}

	sched := bot.NewScheduler(log, &cfg.Scheduler, tasks.RegisterAllTasks(tDeps))
	app := bot.NewBot(log, cfg, db, store, gemClient, tg, sched)

	log.Info("Starting bot...")
	runErr := app.Run(ctx) // Run blocks until context is cancelled or an error occurs
	log.Info("Bot run loop finished. Initiating shutdown...")

	// Check if the error is significant (not just context cancellation)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Error("Bot stopped due to error", "error", runErr)
		// Allow logs to flush before exiting on error
		time.Sleep(time.Second)
		return 1
	}

	log.Info("Bot stopped gracefully.")
	// Allow logs to flush before exiting gracefully
	log.Info("Waiting briefly before exit...")
	time.Sleep(time.Second)
	return 0
}
