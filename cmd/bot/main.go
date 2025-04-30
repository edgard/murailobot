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

	"github.com/edgard/murailobot-go/internal/bot"
	"github.com/edgard/murailobot-go/internal/bot/handlers"
	"github.com/edgard/murailobot-go/internal/bot/tasks"
	"github.com/edgard/murailobot-go/internal/config"
	"github.com/edgard/murailobot-go/internal/database"
	"github.com/edgard/murailobot-go/internal/gemini"
	"github.com/edgard/murailobot-go/internal/logger"
	"github.com/edgard/murailobot-go/internal/telegram"

	_ "modernc.org/sqlite"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	exitCode := run(ctx)
	stop()
	os.Exit(exitCode)
}

func run(ctx context.Context) int {
	// Load configuration
	configPath := flag.String("config", "./config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "path", *configPath, "error", err)
		return 1
	}

	// Initialize logger
	log := logger.NewLogger(cfg.Logger.Level, cfg.Logger.JSON)
	slog.SetDefault(log)
	log.Info("Logger initialized", "level", cfg.Logger.Level, "json", cfg.Logger.JSON)

	// Connect to database
	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		log.Error("Failed to connect to database", "path", cfg.Database.Path, "error", err)
		return 1
	}
	defer database.CloseDB(db)
	store := database.NewStore(db, log)

	// Initialize Gemini client
	gemClient, err := gemini.NewClient(ctx, cfg.Gemini, log)
	if err != nil {
		log.Error("Failed to initialize Gemini client", "error", err)
		return 1
	}

	// Prepare dependencies
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

	// Initialize Telegram bot
	botOpts := []tgbot.Option{
		tgbot.WithMiddlewares(logger.Middleware(log)),
		tgbot.WithDefaultHandler(handlers.NewMentionHandler(hDeps)),
	}
	tg, err := telegram.NewTelegramBot(cfg.Telegram.Token, log, botOpts...)
	if err != nil {
		log.Error("Failed to create Telegram bot", "error", err)
		return 1
	}

	// Retrieve bot info
	cfg.Telegram.BotInfo, err = tg.GetMe(ctx)
	if err != nil {
		log.Error("Failed to get bot info", "error", err)
		return 1
	}
	log.Info("Retrieved bot info", "bot_id", cfg.Telegram.BotInfo.ID, "bot_username", cfg.Telegram.BotInfo.Username)

	// Register Telegram handlers
	cmdHandlers := handlers.RegisterAllCommands(hDeps)
	if err := telegram.RegisterHandlers(tg, log, cmdHandlers); err != nil {
		log.Error("Failed to register Telegram handlers", "error", err)
		return 1
	}

	// Initialize scheduler and bot orchestrator
	sched := bot.NewScheduler(log, &cfg.Scheduler, tasks.RegisterAllTasks(tDeps))
	app := bot.NewBot(log, cfg, db, store, gemClient, tg, sched)

	// Start bot
	log.Info("Starting bot...")
	runErr := app.Run(ctx)
	log.Info("Bot run loop finished. Initiating shutdown...")

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Error("Bot stopped due to error", "error", runErr)
		time.Sleep(time.Second)
		return 1
	}
	log.Info("Bot stopped gracefully.")
	log.Info("Waiting briefly before exit...")
	time.Sleep(time.Second)
	return 0
}
