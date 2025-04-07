package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
)

// Application represents the main application instance
type Application struct {
	db        interfaces.DB
	ai        interfaces.AI
	bot       interfaces.Bot
	scheduler interfaces.Scheduler
	config    *common.Config
}

// New creates a new application instance using the provided interfaces
func New(cfg *common.Config, db interfaces.DB, ai interfaces.AI, bot interfaces.Bot, scheduler interfaces.Scheduler) (*Application, error) {
	if err := configureLogging(cfg.LogLevel); err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInitialization, err)
	}

	app := &Application{
		db:        db,
		ai:        ai,
		bot:       bot,
		scheduler: scheduler,
		config:    cfg,
	}

	if err := app.configureServices(); err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInitialization, err)
	}

	return app, nil
}

// configureServices initializes and configures all services
func (a *Application) configureServices() error {
	// Set dependencies
	if err := a.bot.SetServices(a.ai, a.db, a.scheduler); err != nil {
		return fmt.Errorf("failed to set bot dependencies: %w", err)
	}

	// Configure bot commands and templates
	if err := a.bot.SetCommands(map[string]string{
		"start":    a.config.BotCmdStart,
		"reset":    a.config.BotCmdReset,
		"analyze":  a.config.BotCmdAnalyze,
		"profiles": a.config.BotCmdProfiles,
		"edit":     a.config.BotCmdEditUser,
	}); err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	if err := a.bot.SetTemplates(map[string]string{
		"welcome":         a.config.BotMsgWelcome,
		"unauthorized":    a.config.BotMsgNotAuthorized,
		"empty":           a.config.BotMsgProvideMessage,
		"error":           a.config.BotMsgGeneralError,
		"reset":           a.config.BotMsgHistoryReset,
		"analyzing":       a.config.BotMsgAnalyzing,
		"no_profiles":     a.config.BotMsgNoProfiles,
		"profiles_header": a.config.BotMsgProfilesHeader,
	}); err != nil {
		return fmt.Errorf("failed to set bot templates: %w", err)
	}

	return nil
}

// Start begins the application and listens for errors
func (a *Application) Start(ctx context.Context, errCh chan<- error) error {
	if err := a.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("%w: scheduler failed to start", common.ErrServiceStart)
	}

	go func() {
		if err := a.bot.Start(ctx); err != nil {
			errCh <- fmt.Errorf("%w: %v", common.ErrServiceStart, err)
		}
	}()

	return nil
}

// Stop gracefully shuts down all components in the correct order
func (a *Application) Stop(ctx context.Context) error {
	slog.Info("shutting down application")
	errs := a.stopServices(ctx)

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", common.ErrServiceStop, errs)
	}

	slog.Info("application shutdown completed")
	return nil
}

// stopServices stops all services in the correct order
func (a *Application) stopServices(ctx context.Context) []error {
	done := make(chan struct{})
	errCh := make(chan error, 4)

	go func() {
		defer close(done)

		// 1. Stop AI service first
		if err := a.ai.Stop(); err != nil {
			errCh <- fmt.Errorf("AI shutdown failed: %w", err)
		}

		// 2. Stop bot service next
		if err := a.bot.Stop(); err != nil {
			errCh <- fmt.Errorf("bot shutdown failed: %w", err)
		}

		// 3. Stop scheduler
		if err := a.scheduler.Stop(); err != nil {
			errCh <- fmt.Errorf("scheduler shutdown failed: %w", err)
		}

		// 4. Stop database last
		if err := a.db.Stop(); err != nil {
			errCh <- fmt.Errorf("database shutdown failed: %w", err)
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return []error{fmt.Errorf("%w: shutdown timeout", common.ErrShutdownTimeout)}
	}

	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	return errs
}

// configureLogging sets up structured JSON logging with the specified level
func configureLogging(level string) error {
	logLevel := slog.LevelInfo // default level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	case "info", "":
		// use default
	default:
		return fmt.Errorf("%w: %s", common.ErrInvalidLogLevel, level)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	return nil
}
