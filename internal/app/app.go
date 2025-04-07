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

	// Initialize scheduler
	scheduler, err := services.NewCron(cfg.Cron)
	if err != nil {
		return nil, fmt.Errorf("%w: scheduler initialization failed", common.ErrInitialization)
	}

	// Initialize bot configuration with database and scheduler
	cfg.Telegram.DB = db
	cfg.Telegram.Scheduler = scheduler
	cfg.Telegram.Commands = cfg.Commands
	cfg.Telegram.Templates = cfg.Templates

	// Initialize bot first to get bot info
	bot, err := services.NewTelegram(cfg.Telegram)
	if err != nil {
		return nil, fmt.Errorf("%w: bot initialization failed", common.ErrInitialization)
	}

	// Set bot info in OpenAI config
	cfg.OpenAI.BotID = bot.GetID()
	cfg.OpenAI.BotUserName = bot.GetUserName()
	cfg.OpenAI.BotFirstName = bot.GetFirstName()

	// Initialize AI service
	ai, err := services.NewOpenAI(cfg.OpenAI)
	if err != nil {
		return nil, fmt.Errorf("%w: AI service initialization failed", common.ErrInitialization)
	}

	// Update bot configuration with AI service
	cfg.Telegram.AI = ai

	// Re-initialize Telegram service with complete configuration
	bot, err = services.NewTelegram(cfg.Telegram)
	if err != nil {
		return nil, fmt.Errorf("%w: bot re-initialization failed", common.ErrInitialization)
	}

	return &Application{
		db:        db,
		ai:        ai,
		bot:       bot,
		scheduler: scheduler,
	}, nil
}

// Start begins the application and listens for errors
func (a *Application) Start(ctx context.Context, errCh chan<- error) error {
	// Start scheduler with parent context
	if err := a.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("%w: scheduler failed to start", common.ErrServiceStart)
	}

	// Start bot with error monitoring and parent context
	go func() {
		if err := a.bot.Start(ctx); err != nil {
			errCh <- fmt.Errorf("%w: %v", common.ErrServiceStart, err)
		}
	}()

	return nil
}

// Stop gracefully shuts down all components in the correct order:
// 1. AI service (stop first to prevent new message processing)
// 2. Bot service (stop next to prevent new messages)
// 3. Scheduler (stop next to prevent scheduled tasks)
// 4. Database (stop last as other services might need it during shutdown)
func (a *Application) Stop(ctx context.Context) error {
	slog.Info("shutting down application")

	// Create error channel for shutdown errors
	errCh := make(chan error, 4) // Buffer size matches number of services
	done := make(chan struct{})

	// Shutdown components in the correct order
	go func() {
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

		close(done)
	}()

	// Wait for shutdown with timeout
	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("%w: shutdown timeout", common.ErrShutdownTimeout)
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

	slog.Info("application shutdown completed")
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
