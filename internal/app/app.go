package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/edgard/murailobot/internal/services"
)

// Config holds application configuration
type Config struct {
	LogLevel string

	// Service configurations
	DB        services.SQLConfig
	OpenAI    services.OpenAIConfig
	Cron      services.CronConfig
	Telegram  services.TelegramConfig
	Commands  map[string]string
	Templates map[string]string
}

// LoadFromConfig creates a new Config from the given common.Config
func LoadFromConfig(cfg *common.Config) Config {
	// Build command map
	commands := map[string]string{
		"start":    cfg.BotCmdStart,
		"reset":    cfg.BotCmdReset,
		"analyze":  cfg.BotCmdAnalyze,
		"profiles": cfg.BotCmdProfiles,
		"edit":     cfg.BotCmdEditUser,
	}

	// Build template map
	templates := map[string]string{
		"welcome":         cfg.BotMsgWelcome,
		"unauthorized":    cfg.BotMsgNotAuthorized,
		"empty":           cfg.BotMsgProvideMessage,
		"error":           cfg.BotMsgGeneralError,
		"reset":           cfg.BotMsgHistoryReset,
		"analyzing":       cfg.BotMsgAnalyzing,
		"no_profiles":     cfg.BotMsgNoProfiles,
		"profiles_header": cfg.BotMsgProfilesHeader,
	}

	return Config{
		LogLevel: cfg.LogLevel,
		DB: services.SQLConfig{
			Path: cfg.DBPath,
		},
		OpenAI: services.OpenAIConfig{
			Token:              cfg.AIToken,
			BaseURL:            cfg.AIBaseURL,
			Model:              cfg.AIModel,
			MaxTokens:          cfg.AIMaxContextTokens,
			Temperature:        cfg.AITemperature,
			Timeout:            cfg.AITimeout,
			Instruction:        cfg.AIInstruction,
			ProfileInstruction: cfg.AIProfileInstruction,
		},
		Cron: services.CronConfig{
			TimeZone: "UTC", // Default to UTC
		},
		Telegram: services.TelegramConfig{
			Token:          cfg.BotToken,
			AdminID:        cfg.BotAdminID,
			MaxContextSize: cfg.AIMaxContextTokens / 2, // Use half of AI context for history
		},
		Commands:  commands,
		Templates: templates,
	}
}

// Application represents the main application instance
type Application struct {
	db        interfaces.DB
	ai        interfaces.AI
	bot       interfaces.Bot
	scheduler interfaces.Scheduler
}

// New creates a new application instance
func New(cfg Config) (*Application, error) {
	// Configure logging
	if err := configureLogging(cfg.LogLevel); err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInitialization, err)
	}

	// Initialize database
	db, err := services.NewSQL(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("%w: database initialization failed", common.ErrInitialization)
	}

	// Initialize AI service
	cfg.OpenAI.Bot = nil // Will be set after bot initialization
	ai, err := services.NewOpenAI(cfg.OpenAI)
	if err != nil {
		return nil, fmt.Errorf("%w: AI service initialization failed", common.ErrInitialization)
	}

	// Initialize scheduler
	scheduler, err := services.NewCron(cfg.Cron)
	if err != nil {
		return nil, fmt.Errorf("%w: scheduler initialization failed", common.ErrInitialization)
	}

	// Initialize bot configuration
	cfg.Telegram.AI = ai
	cfg.Telegram.DB = db
	cfg.Telegram.Scheduler = scheduler
	cfg.Telegram.Commands = cfg.Commands
	cfg.Telegram.Templates = cfg.Templates

	// Initialize bot
	bot, err := services.NewTelegram(cfg.Telegram)
	if err != nil {
		return nil, fmt.Errorf("%w: bot initialization failed", common.ErrInitialization)
	}

	// Set bot reference in OpenAI service
	cfg.OpenAI.Bot = bot

	return &Application{
		db:        db,
		ai:        ai,
		bot:       bot,
		scheduler: scheduler,
	}, nil
}

// Start begins the application and listens for errors
func (a *Application) Start(errCh chan<- error) error {
	// Start scheduler
	if err := a.scheduler.Start(context.Background()); err != nil {
		return fmt.Errorf("%w: scheduler failed to start", common.ErrServiceStart)
	}

	// Start bot with error monitoring
	go func() {
		if err := a.bot.Start(context.Background()); err != nil {
			errCh <- fmt.Errorf("%w: %v", common.ErrServiceStart, err)
		}
	}()

	return nil
}

// Stop gracefully shuts down all components
func (a *Application) Stop(ctx context.Context) error {
	slog.Info("shutting down application")

	// Create error channel for parallel shutdown
	errCh := make(chan error, 3)
	done := make(chan struct{})

	// Shutdown components in parallel
	go func() {
		if err := a.bot.Stop(); err != nil {
			errCh <- fmt.Errorf("%w: bot shutdown failed", common.ErrServiceStop)
		}

		if err := a.scheduler.Stop(); err != nil {
			errCh <- fmt.Errorf("%w: scheduler shutdown failed", common.ErrServiceStop)
		}

		// Close database last
		if db, ok := a.db.(*services.SQL); ok {
			if err := db.Close(); err != nil {
				errCh <- fmt.Errorf("%w: database shutdown failed", common.ErrServiceStop)
			}
		}

		close(done)
	}()

	// Wait for shutdown with timeout
	select {
	case <-done:
	case <-ctx.Done():
		return common.ErrShutdownTimeout
	}

	// Collect any errors that occurred during shutdown
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", common.ErrServiceStop, errs)
	}

	return nil
}

func configureLogging(level string) error {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	case "info", "":
		logLevel = slog.LevelInfo
	default:
		return fmt.Errorf("%w: %s", common.ErrInvalidLogLevel, level)
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})

	slog.SetDefault(slog.New(handler))

	return nil
}
