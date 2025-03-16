// Package app provides application orchestration and component lifecycle management
// for the MurailoBot Telegram bot.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/bot"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
)

// App represents the application and its components.
type App struct {
	config    *config.Config
	db        *db.DB
	ai        *ai.Client
	scheduler *utils.Scheduler
	bot       *bot.Bot
}

// New creates a new application instance with configured components.
// It initializes logging, loads configuration, and sets up all required
// services. Returns an error if any component initialization fails.
func New() (*App, error) {
	if err := configureLogging(nil); err != nil {
		return nil, fmt.Errorf("failed to setup initial logging: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := configureLogging(cfg); err != nil {
		return nil, fmt.Errorf("failed to configure logging: %w", err)
	}

	sched, err := utils.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	database, err := db.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	aiClient, err := ai.New(cfg, database)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AI client: %w", err)
	}

	botInstance, err := bot.New(cfg, database, aiClient, sched)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bot: %w", err)
	}

	return &App{
		config:    cfg,
		db:        database,
		ai:        aiClient,
		scheduler: sched,
		bot:       botInstance,
	}, nil
}

// Start launches the application by starting the Telegram bot.
// It takes an error channel to report runtime errors that occur after startup.
// Returns an error if the bot fails to start.
func (a *App) Start(errCh chan<- error) error {
	if err := a.bot.Start(errCh); err != nil {
		return fmt.Errorf("failed to start bot: %w", err)
	}

	return nil
}

// Stop gracefully shuts down all application components.
// It attempts to stop the bot, scheduler, and close the database connection,
// and returns an error if any component fails to shut down properly.
// The context allows for timeout control during shutdown.
func (a *App) Stop(ctx context.Context) error {
	var errs []error

	// Create a channel to collect errors from shutdown operations
	errCh := make(chan error, 3)

	// Create a done channel to signal completion of all shutdown tasks
	done := make(chan struct{})

	// Start shutdown operations concurrently
	go func() {
		if err := a.bot.Stop(); err != nil {
			slog.Error("failed to stop bot", "error", err)
			errCh <- fmt.Errorf("bot stop: %w", err)
		}

		if err := a.scheduler.Stop(); err != nil {
			slog.Error("failed to stop scheduler", "error", err)
			errCh <- fmt.Errorf("scheduler stop: %w", err)
		}

		if err := a.db.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
			errCh <- fmt.Errorf("database close: %w", err)
		}

		close(done)
	}()

	// Wait for either completion or timeout
	select {
	case <-ctx.Done():
		// Context timed out
		return fmt.Errorf("shutdown timed out: %v", ctx.Err())
	case <-done:
		// Collect any errors
		close(errCh)
		for err := range errCh {
			errs = append(errs, err)
		}

		if len(errs) > 0 {
			return fmt.Errorf("errors during shutdown: %v", errs)
		}
		return nil
	}
}

func configureLogging(cfg *config.Config) error {
	level := slog.LevelInfo

	if cfg != nil {
		switch cfg.LogLevel {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		case "info":
		default:
			return os.ErrInvalid
		}
	}

	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
