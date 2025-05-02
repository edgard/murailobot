// Package bot implements the core bot functionality, lifecycle management,
// and component orchestration for the MurailoBot Telegram bot.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tgbot "github.com/go-telegram/bot"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
	"github.com/edgard/murailobot/internal/gemini"
)

// Bot represents the main bot application and manages its components' lifecycle.
type Bot struct {
	logger       *slog.Logger
	cfg          *config.Config
	db           *sqlx.DB
	store        database.Store
	geminiClient gemini.Client
	tgBot        *tgbot.Bot
	scheduler    *Scheduler
}

// NewBot creates a new instance of the bot with all required dependencies.
// It initializes the bot with a logger, configuration, database, store, AI client,
// Telegram client, and scheduler for managing scheduled tasks.
func NewBot(
	logger *slog.Logger,
	cfg *config.Config,
	db *sqlx.DB,
	store database.Store,
	geminiClient gemini.Client,
	tgBot *tgbot.Bot,
	scheduler *Scheduler,
) *Bot {
	return &Bot{
		logger:       logger.With("component", "bot_orchestrator"),
		cfg:          cfg,
		db:           db,
		store:        store,
		geminiClient: geminiClient,
		tgBot:        tgBot,
		scheduler:    scheduler,
	}
}

// Run starts the bot and all its components, handling graceful shutdown on context cancellation.
// It returns an error if any component fails during startup or execution.
func (b *Bot) Run(ctx context.Context) error {
	b.logger.Info("Starting bot orchestrator...")

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		b.logger.Info("Starting Telegram bot listener...")

		b.tgBot.Start(gCtx)
		b.logger.Info("Telegram bot listener stopped.")

		if gCtx.Err() == nil {
			b.logger.Warn("Telegram bot listener stopped unexpectedly without context cancellation.")

			return fmt.Errorf("telegram listener stopped unexpectedly")
		}
		return nil
	})

	g.Go(func() error {
		b.logger.Info("Starting scheduler...")
		if err := b.scheduler.Start(); err != nil {
			b.logger.Error("Failed to start scheduler", "error", err)
			return fmt.Errorf("failed to start scheduler: %w", err)
		}

		<-gCtx.Done()
		b.logger.Info("Shutdown signal received, stopping scheduler...")

		if err := b.scheduler.Stop(); err != nil {
			b.logger.Error("Error stopping scheduler", "error", err)
		}

		return nil
	})

	b.logger.Info("Bot orchestrator running. Waiting for shutdown signal or error...")
	err := g.Wait()

	if err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error("Bot orchestrator stopped due to error", "error", err)
		return err
	}

	b.logger.Info("Bot orchestrator stopped gracefully.")
	return nil
}
