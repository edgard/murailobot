// Package di provides dependency injection providers for MurailoBot.
package di

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/adapter/ai/openai"
	"github.com/edgard/murailobot/internal/adapter/chat/telegram"
	"github.com/edgard/murailobot/internal/adapter/scheduler/cron"
	"github.com/edgard/murailobot/internal/adapter/store/sqlite"
	"github.com/edgard/murailobot/internal/infrastructure/config"
	"github.com/edgard/murailobot/internal/infrastructure/logging"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// ProvideLogger creates and provides the application logger.
func ProvideLogger() (*zap.Logger, error) {
	// Create a bootstrap logger
	logger := zap.NewExample().Named("app")
	return logger, nil
}

// ProvideConfig loads and provides the application configuration.
func ProvideConfig(logger *zap.Logger) (*config.Config, error) {
	configLogger := logger.Named("config")

	// Load configuration from default path
	cfg, err := config.LoadConfig(configLogger)
	if err != nil {
		return nil, err
	}

	// Configure logger based on loaded config using the function from the logging package
	if err := logging.ConfigureLogger(logger, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure logger: %w", err)
	}

	return cfg, nil
}

// ProvideStore creates and provides the database store implementation.
func ProvideStore(cfg *config.Config, logger *zap.Logger) (store.Store, error) {
	storeLogger := logging.Named(logger, "store")
	return sqlite.NewStore(cfg, storeLogger)
}

// ProvideAIService creates and provides the AI service implementation.
func ProvideAIService(cfg *config.Config, store store.Store, logger *zap.Logger) (ai.Service, error) {
	aiLogger := logging.Named(logger, "ai")
	return openai.NewAIService(cfg, store, aiLogger)
}

// ProvideSchedulerService creates and provides the scheduler service implementation.
func ProvideSchedulerService(logger *zap.Logger) (scheduler.Service, error) {
	schedulerLogger := logging.Named(logger, "scheduler")
	return cron.NewScheduler(schedulerLogger)
}

// ProvideChatService creates and provides the chat service implementation.
func ProvideChatService(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	scheduler scheduler.Service,
	logger *zap.Logger,
) (chat.Service, error) {
	chatLogger := logging.Named(logger, "chat")
	return telegram.NewChatService(cfg, store, aiService, scheduler, chatLogger)
}

// ProvideApp creates the main application container.
func ProvideApp(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	chatService chat.Service,
	schedulerService scheduler.Service,
	logger *zap.Logger,
) *AppComponents {
	return &AppComponents{
		Config:      cfg,
		Store:       store,
		AIService:   aiService,
		ChatService: chatService,
		Scheduler:   schedulerService,
		Logger:      logger,
	}
}

// AppComponents holds references to all application components.
// This is used to avoid circular dependency issues with the main package.
type AppComponents struct {
	Config      *config.Config
	Store       store.Store
	AIService   ai.Service
	ChatService chat.Service
	Scheduler   scheduler.Service
	Logger      *zap.Logger
}
