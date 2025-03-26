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
// It loads configuration and sets up all required services.
// A default logger should be initialized before calling this function.
// Returns an error if any component initialization fails.
func New() (*App, error) {
	// Load configuration
	slog.Debug("initializing application")

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Configure logging with application settings from config
	if err := configureLogging(cfg); err != nil {
		return nil, fmt.Errorf("failed to configure logging: %w", err)
	}

	// Configuration loaded message now comes from config.Load() - no need to log it again here

	// Initialize components
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

	slog.Info("application initialized")

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
	slog.Debug("starting application")

	if err := a.bot.Start(errCh); err != nil {
		return fmt.Errorf("failed to start bot: %w", err)
	}

	// We're already logging "bot started and processing updates" in bot.go
	// No need for duplicate "application started" message
	return nil
}

// Stop gracefully shuts down all application components.
// It attempts to stop the bot, scheduler, and close the database connection,
// and returns an error if any component fails to shut down properly.
// The context allows for timeout control during shutdown.
func (a *App) Stop(ctx context.Context) error {
	slog.Info("shutting down application")

	var errs []error

	// Create channels for error collection and completion signaling
	errCh := make(chan error, 3)
	done := make(chan struct{})

	// Start shutdown operations concurrently
	go func() {
		// Stop components in order of dependency
		if err := a.bot.Stop(); err != nil {
			errCh <- fmt.Errorf("bot stop: %w", err)
		}

		if err := a.scheduler.Stop(); err != nil {
			errCh <- fmt.Errorf("scheduler stop: %w", err)
		}

		if err := a.db.Close(); err != nil {
			errCh <- fmt.Errorf("database close: %w", err)
		}

		close(done)
	}()

	// Wait for either completion or timeout
	select {
	case <-ctx.Done():
		return fmt.Errorf("shutdown timed out: %w", ctx.Err())
	case <-done:
		// Collect any errors
		close(errCh)

		for err := range errCh {
			errs = append(errs, err)
		}

		if len(errs) > 0 {
			return fmt.Errorf("errors during shutdown: %v", errs)
		}

		slog.Info("application shutdown completed")

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

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	return nil
}
