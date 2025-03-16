// Package app provides application orchestration and component lifecycle management
// for the MurailoBot Telegram bot.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	startTime := time.Now()
	slog.Info("initializing application")

	// Track component initialization times
	componentTimes := make(map[string]int64)

	// Configure initial logging
	loggingStartTime := time.Now()
	if err := configureLogging(nil); err != nil {
		slog.Error("failed to setup initial logging",
			"error", err,
			"duration_ms", time.Since(loggingStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to setup initial logging: %w", err)
	}
	componentTimes["initial_logging"] = time.Since(loggingStartTime).Milliseconds()

	// Load configuration
	configStartTime := time.Now()
	slog.Info("loading configuration")
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration",
			"error", err,
			"duration_ms", time.Since(configStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	componentTimes["config_loading"] = time.Since(configStartTime).Milliseconds()

	slog.Info("configuration loaded",
		"log_level", cfg.LogLevel,
		"ai_model", cfg.AIModel,
		"duration_ms", componentTimes["config_loading"])

	// Configure logging with loaded config
	finalLoggingStartTime := time.Now()
	if err := configureLogging(cfg); err != nil {
		slog.Error("failed to configure logging",
			"error", err,
			"duration_ms", time.Since(finalLoggingStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to configure logging: %w", err)
	}
	componentTimes["final_logging"] = time.Since(finalLoggingStartTime).Milliseconds()
	slog.Info("logging configured",
		"level", cfg.LogLevel,
		"duration_ms", componentTimes["final_logging"])

	// Initialize scheduler
	schedulerStartTime := time.Now()
	slog.Info("initializing scheduler")
	sched, err := utils.NewScheduler()
	if err != nil {
		slog.Error("failed to initialize scheduler",
			"error", err,
			"duration_ms", time.Since(schedulerStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to initialize scheduler: %w", err)
	}
	componentTimes["scheduler"] = time.Since(schedulerStartTime).Milliseconds()
	slog.Info("scheduler initialized",
		"duration_ms", componentTimes["scheduler"])

	// Initialize database
	dbStartTime := time.Now()
	slog.Info("initializing database")
	database, err := db.New(cfg)
	if err != nil {
		slog.Error("failed to initialize database",
			"error", err,
			"duration_ms", time.Since(dbStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	componentTimes["database"] = time.Since(dbStartTime).Milliseconds()
	slog.Info("database initialized",
		"duration_ms", componentTimes["database"])

	// Initialize AI client
	aiStartTime := time.Now()
	slog.Info("initializing AI client")
	aiClient, err := ai.New(cfg, database)
	if err != nil {
		slog.Error("failed to initialize AI client",
			"error", err,
			"duration_ms", time.Since(aiStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to initialize AI client: %w", err)
	}
	componentTimes["ai_client"] = time.Since(aiStartTime).Milliseconds()
	slog.Info("AI client initialized",
		"duration_ms", componentTimes["ai_client"])

	// Initialize bot
	botStartTime := time.Now()
	slog.Info("initializing bot")
	botInstance, err := bot.New(cfg, database, aiClient, sched)
	if err != nil {
		slog.Error("failed to initialize bot",
			"error", err,
			"duration_ms", time.Since(botStartTime).Milliseconds())
		return nil, fmt.Errorf("failed to initialize bot: %w", err)
	}
	componentTimes["bot"] = time.Since(botStartTime).Milliseconds()
	slog.Info("bot initialized",
		"duration_ms", componentTimes["bot"])

	// Log total initialization time and component breakdown
	totalDuration := time.Since(startTime)
	slog.Info("application initialization complete",
		"total_duration_ms", totalDuration.Milliseconds(),
		"component_times", componentTimes)

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
	startTime := time.Now()
	slog.Info("starting application",
		"ai_model", a.config.AIModel)

	slog.Debug("starting bot component")
	botStartTime := time.Now()

	if err := a.bot.Start(errCh); err != nil {
		slog.Error("failed to start bot",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return fmt.Errorf("failed to start bot: %w", err)
	}

	botStartDuration := time.Since(botStartTime)
	totalStartDuration := time.Since(startTime)

	slog.Info("application started successfully",
		"bot_start_duration_ms", botStartDuration.Milliseconds(),
		"total_duration_ms", totalStartDuration.Milliseconds())
	return nil
}

// Stop gracefully shuts down all application components.
// It attempts to stop the bot, scheduler, and close the database connection,
// and returns an error if any component fails to shut down properly.
// The context allows for timeout control during shutdown.
func (a *App) Stop(ctx context.Context) error {
	shutdownStartTime := time.Now()
	slog.Info("initiating application shutdown")

	var errs []error

	// Create a channel to collect errors from shutdown operations
	errCh := make(chan error, 3)

	// Create a done channel to signal completion of all shutdown tasks
	done := make(chan struct{})

	// Start shutdown operations concurrently
	go func() {
		// Track component shutdown times
		componentTimes := make(map[string]int64)

		// Stop bot
		slog.Info("stopping bot")
		botStopStart := time.Now()
		if err := a.bot.Stop(); err != nil {
			slog.Error("failed to stop bot",
				"error", err,
				"duration_ms", time.Since(botStopStart).Milliseconds())
			errCh <- fmt.Errorf("bot stop: %w", err)
		} else {
			botStopDuration := time.Since(botStopStart)
			componentTimes["bot"] = botStopDuration.Milliseconds()
			slog.Info("bot stopped successfully",
				"duration_ms", botStopDuration.Milliseconds())
		}

		// Stop scheduler
		slog.Info("stopping scheduler")
		schedulerStopStart := time.Now()
		if err := a.scheduler.Stop(); err != nil {
			slog.Error("failed to stop scheduler",
				"error", err,
				"duration_ms", time.Since(schedulerStopStart).Milliseconds())
			errCh <- fmt.Errorf("scheduler stop: %w", err)
		} else {
			schedulerStopDuration := time.Since(schedulerStopStart)
			componentTimes["scheduler"] = schedulerStopDuration.Milliseconds()
			slog.Info("scheduler stopped successfully",
				"duration_ms", schedulerStopDuration.Milliseconds())
		}

		// Close database
		slog.Info("closing database connection")
		dbCloseStart := time.Now()
		if err := a.db.Close(); err != nil {
			slog.Error("failed to close database",
				"error", err,
				"duration_ms", time.Since(dbCloseStart).Milliseconds())
			errCh <- fmt.Errorf("database close: %w", err)
		} else {
			dbCloseDuration := time.Since(dbCloseStart)
			componentTimes["database"] = dbCloseDuration.Milliseconds()
			slog.Info("database connection closed successfully",
				"duration_ms", dbCloseDuration.Milliseconds())
		}

		close(done)
	}()

	// Wait for either completion or timeout
	select {
	case <-ctx.Done():
		// Context timed out
		timeoutDuration := time.Since(shutdownStartTime)
		slog.Error("shutdown timed out",
			"error", ctx.Err(),
			"timeout_after_ms", timeoutDuration.Milliseconds())
		return fmt.Errorf("shutdown timed out after %v: %v", timeoutDuration, ctx.Err())
	case <-done:
		// Collect any errors
		close(errCh)
		for err := range errCh {
			errs = append(errs, err)
		}

		totalShutdownDuration := time.Since(shutdownStartTime)

		if len(errs) > 0 {
			slog.Error("shutdown completed with errors",
				"error_count", len(errs),
				"total_duration_ms", totalShutdownDuration.Milliseconds())
			return fmt.Errorf("errors during shutdown: %v", errs)
		}

		slog.Info("application shutdown completed successfully",
			"total_duration_ms", totalShutdownDuration.Milliseconds())
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
