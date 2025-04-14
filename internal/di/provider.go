// Package di provides dependency injection providers for MurailoBot.
package di

import (
	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/adapter/ai/openai"
	"github.com/edgard/murailobot/internal/adapter/chat/telegram"
	"github.com/edgard/murailobot/internal/adapter/scheduler/cron"
	"github.com/edgard/murailobot/internal/adapter/store/sqlite"
	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// ProvideConfig loads and provides the application configuration.
func ProvideConfig(logger *zap.Logger) (*config.Config, error) {
	configLogger := logger.Named("config")
	return config.Load("config.yaml", configLogger)
}

// ProvideStore creates and provides the database store implementation.
func ProvideStore(cfg *config.Config, logger *zap.Logger) (store.Store, error) {
	storeLogger := logger.Named("store")
	return sqlite.NewStore(cfg, storeLogger)
}

// ProvideAIService creates and provides the AI service implementation.
func ProvideAIService(cfg *config.Config, store store.Store, logger *zap.Logger) (ai.Service, error) {
	aiLogger := logger.Named("ai")
	return openai.NewAIService(cfg, store, aiLogger)
}

// ProvideSchedulerService creates and provides the scheduler service implementation.
func ProvideSchedulerService(logger *zap.Logger) (scheduler.Service, error) {
	schedulerLogger := logger.Named("scheduler")
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
	chatLogger := logger.Named("chat")
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
