// Package fx provides dependency injection providers for MurailoBot.
package fx

import (
	"go.uber.org/fx"

	"github.com/edgard/murailobot/internal/adapter/ai/openai"
	"github.com/edgard/murailobot/internal/adapter/chat/telegram"
	"github.com/edgard/murailobot/internal/adapter/scheduler/cron"
	"github.com/edgard/murailobot/internal/adapter/store/sqlite"
	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/domain/service"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// ProvideConfig loads and provides the application configuration.
func ProvideConfig() (*config.Config, error) {
	return config.Load("config.yaml")
}

// ProvideStore creates and provides the database store implementation.
func ProvideStore(cfg *config.Config) (store.Store, error) {
	return sqlite.NewStore(cfg)
}

// ProvideAIService creates and provides the AI service implementation.
func ProvideAIService(cfg *config.Config, store store.Store) (ai.Service, error) {
	return openai.NewAIService(cfg, store)
}

// ProvideSchedulerService creates and provides the scheduler service implementation.
func ProvideSchedulerService() (scheduler.Service, error) {
	return cron.NewScheduler()
}

// ProvideChatService creates and provides the chat service implementation.
func ProvideChatService(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	scheduler scheduler.Service,
) (chat.Service, error) {
	return telegram.NewChatService(cfg, store, aiService, scheduler)
}

// ProvideDomainService creates the main domain service for the application.
func ProvideDomainService(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	chatService chat.Service,
	schedulerService scheduler.Service,
) *service.ChatService {
	return service.NewChatService(cfg, store, aiService, chatService, schedulerService)
}

// DependencyProviders groups all the provider functions for cleaner module definitions.
var DependencyProviders = fx.Provide(
	ProvideConfig,
	fx.Annotate(
		ProvideStore,
		fx.As(new(store.Store)),
	),
	fx.Annotate(
		ProvideAIService,
		fx.As(new(ai.Service)),
	),
	fx.Annotate(
		ProvideSchedulerService,
		fx.As(new(scheduler.Service)),
	),
	fx.Annotate(
		ProvideChatService,
		fx.As(new(chat.Service)),
	),
	ProvideDomainService,
)
