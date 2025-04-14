// Package di provides dependency injection modules using Uber's fx.
package di

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// ServiceParams provides a grouped parameter object for service creation
// following the pattern in the migration document
type ServiceParams struct {
	fx.In

	Config    *config.Config
	Store     store.Store
	AI        ai.Service
	Chat      chat.Service
	Scheduler scheduler.Service
	Logger    *zap.Logger
}

// ConfigModule provides application configuration
var ConfigModule = fx.Module("config",
	fx.Provide(
		ProvideConfig,
	),
)

// StoreModule provides the database store implementation
var StoreModule = fx.Module("store",
	fx.Provide(
		fx.Annotate(
			ProvideStore,
			fx.As(new(store.Store)),
		),
	),
	fx.Invoke(func(lc fx.Lifecycle, store store.Store, logger *zap.Logger) {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				logger.Info("closing database connection")
				return store.Close()
			},
		})
	}),
)

// AIModule provides the AI service implementation
var AIModule = fx.Module("ai",
	fx.Provide(
		fx.Annotate(
			ProvideAIService,
			fx.As(new(ai.Service)),
		),
	),
)

// SchedulerModule provides the scheduler service implementation
var SchedulerModule = fx.Module("scheduler",
	fx.Provide(
		fx.Annotate(
			ProvideSchedulerService,
			fx.As(new(scheduler.Service)),
		),
	),
	fx.Invoke(func(lc fx.Lifecycle, scheduler scheduler.Service, logger *zap.Logger) {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				logger.Info("stopping scheduler")
				return scheduler.Stop()
			},
		})
	}),
)

// ChatModule provides the chat service implementation
var ChatModule = fx.Module("chat",
	fx.Provide(
		fx.Annotate(
			ProvideChatService,
			fx.As(new(chat.Service)),
		),
	),
	fx.Invoke(func(lc fx.Lifecycle, chatService chat.Service, logger *zap.Logger) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				logger.Info("starting chat service")
				go func() {
					if err := chatService.Start(); err != nil {
						logger.Error("chat service error", zap.Error(err))
						// Consider using fx.Shutdown or similar if this error should
						// terminate the application
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				logger.Info("stopping chat service")
				return chatService.Stop()
			},
		})
	}),
)

// ApplicationModule provides the main application service
var ApplicationModule = fx.Module("application",
	fx.Provide(
		ProvideApp,
	),
)

// RootModule is the main application module that combines all submodules
var RootModule = fx.Module("root",
	ConfigModule,
	StoreModule,
	AIModule,
	SchedulerModule,
	ChatModule,
	ApplicationModule,
)
