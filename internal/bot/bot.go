// Package bot provides core orchestration and scheduler for the Telegram bot.
package bot

import (
	"context"
	"errors"
	"fmt" // Import fmt for error formatting
	"log/slog"

	tgbot "github.com/go-telegram/bot" // Alias tgbot
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup" // Use errgroup for managing goroutines

	"github.com/edgard/murailobot-go/internal/config"
	"github.com/edgard/murailobot-go/internal/database"
	"github.com/edgard/murailobot-go/internal/gemini"
)

// Bot orchestrates the main components of the application.
type Bot struct {
	logger       *slog.Logger
	cfg          *config.Config
	db           *sqlx.DB // Keep DB connection for potential direct needs or easier closing management in main
	store        database.Store
	geminiClient gemini.Client
	tgBot        *tgbot.Bot // Renamed from gobot.Bot for clarity
	scheduler    *Scheduler // Use the custom Scheduler wrapper
}

// NewBot creates a new Bot instance.
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

// Run starts the bot's main components (Telegram listener, scheduler)
// and waits for a shutdown signal via the context.
func (b *Bot) Run(ctx context.Context) error {
	b.logger.Info("Starting bot orchestrator...")

	// Use errgroup to manage lifecycles of concurrent components
	g, gCtx := errgroup.WithContext(ctx)

	// Start the Telegram bot listener
	g.Go(func() error {
		b.logger.Info("Starting Telegram bot listener...")
		// tgBot.Start blocks until the context is cancelled or an error occurs
		b.tgBot.Start(gCtx)
		b.logger.Info("Telegram bot listener stopped.")
		// If Start stops due to context cancellation, it returns nil or context.Canceled.
		// We only care if it stops unexpectedly.
		if gCtx.Err() == nil {
			b.logger.Warn("Telegram bot listener stopped unexpectedly without context cancellation.")
			// Return an error to signal the errgroup that something went wrong.
			return fmt.Errorf("telegram listener stopped unexpectedly")
		}
		return nil // Return nil on expected shutdown (context cancelled)
	})

	// Start the scheduler management goroutine
	g.Go(func() error {
		b.logger.Info("Starting scheduler...")
		// Start the scheduler (schedules jobs and starts ticker)
		if err := b.scheduler.Start(); err != nil {
			b.logger.Error("Failed to start scheduler", "error", err)
			return fmt.Errorf("failed to start scheduler: %w", err) // Propagate error to errgroup
		}
		// The scheduler.Start() function now handles its own completion logging

		// Wait for the context to be done to trigger shutdown.
		<-gCtx.Done() // Wait for shutdown signal from context cancellation
		b.logger.Info("Shutdown signal received, stopping scheduler...")

		// Stop the scheduler gracefully. gocron's Shutdown is blocking.
		if err := b.scheduler.Stop(); err != nil {
			b.logger.Error("Error stopping scheduler", "error", err)
			// Even if stopping fails, we don't necessarily want to return an error
			// during graceful shutdown, but logging it is important.
		}
		// The scheduler.Stop() function now handles its own completion logging
		return nil // Return nil after attempting shutdown
	})

	// Wait for the first error or for all goroutines to finish
	b.logger.Info("Bot orchestrator running. Waiting for shutdown signal or error...")
	err := g.Wait()

	// Check the reason for stopping
	// Ignore context.Canceled errors as they indicate graceful shutdown.
	if err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error("Bot orchestrator stopped due to error", "error", err)
		return err // Propagate the actual error
	}

	b.logger.Info("Bot orchestrator stopped gracefully.")
	return nil
}

// Note: Closing of DB and Gemini client is handled by defer in main.go
// after Run() returns.
