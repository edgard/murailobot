package telegram

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
)

const componentName = "telegram"

func New(cfg *config.Config, database db.Database, aiService ai.Service) (BotService, error) {
	if cfg == nil {
		return nil, utils.NewError(componentName, utils.ErrValidation, "config is nil", utils.CategoryValidation, nil)
	}
	if database == nil {
		return nil, utils.NewError(componentName, utils.ErrValidation, "database is nil", utils.CategoryValidation, nil)
	}
	if aiService == nil {
		return nil, utils.NewError(componentName, utils.ErrValidation, "AI service is nil", utils.CategoryValidation, nil)
	}

	breaker := utils.NewCircuitBreaker(utils.CircuitBreakerConfig{
		Name: "telegram-api",
		OnStateChange: func(name string, from, to utils.CircuitState) {
			utils.InfoLog(componentName, "Telegram API circuit breaker state changed",
				utils.KeyName, name,
				utils.KeyFrom, from.String(),
				utils.KeyTo, to.String(),
				utils.KeyAction, "circuit_breaker_change",
				utils.KeyType, "telegram_api")
		},
	})

	var tgBot *gotgbot.Bot
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			var err error
			tgBot, err = gotgbot.NewBot(cfg.Telegram.Token, nil)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to create bot", utils.CategoryOperation, err)
			}
			return nil
		}, utils.DefaultRetryConfig())
	})

	if err != nil {
		return nil, err
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			utils.ErrorLog(componentName, "error handling update", err,
				utils.KeyRequestID, ctx.Update.UpdateId,
				utils.KeyAction, "handle_update",
				utils.KeyType, "telegram_update")
			return ext.DispatcherActionNoop
		},
	})

	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{
		ErrorLog: nil,
	})

	svc := &bot{
		Bot:     tgBot,
		updater: updater,
		db:      database,
		ai:      aiService,
		cfg:     cfg,
		breaker: breaker,
	}

	dispatcher.AddHandlerToGroup(newCommandHandler(svc), 0)

	return svc, nil
}

func (b *bot) Start(ctx context.Context) error {
	if err := b.updater.StartPolling(b.Bot, &ext.PollingOpts{
		DropPendingUpdates: b.cfg.Telegram.Polling.DropPendingUpdates,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: int64(b.cfg.Telegram.Polling.Timeout.Seconds()),
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: b.cfg.Telegram.Polling.RequestTimeout,
			},
		},
	}); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to start polling", utils.CategoryOperation, err)
	}

	utils.InfoLog(componentName, "bot started",
		utils.KeyAction, "start",
		utils.KeyType, "telegram_bot")

	<-ctx.Done()
	utils.InfoLog(componentName, "shutting down bot",
		utils.KeyAction, "shutdown",
		utils.KeyType, "telegram_bot")

	if err := b.updater.Stop(); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to stop updater", utils.CategoryOperation, err)
	}

	return nil
}

func (b *bot) Stop() error {
	return b.updater.Stop()
}

func (b *bot) SendTypingAction(chatID int64) error {
	utils.DebugLog(componentName, "sending typing action",
		utils.KeyAction, "send_typing",
		utils.KeyType, "telegram_api",
		utils.KeyRequestID, chatID)

	_, err := b.Bot.SendChatAction(chatID, "typing", nil)
	if err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to send typing action", utils.CategoryOperation, err)
	}
	return nil
}

// SendContinuousTyping refreshes typing status at configured intervals
// until context cancellation. Used during long operations like AI generation.
func (b *bot) SendContinuousTyping(ctx context.Context, bot *gotgbot.Bot, chatID int64) {
	_, err := bot.SendChatAction(chatID, "typing", &gotgbot.SendChatActionOpts{
		RequestOpts: &gotgbot.RequestOpts{
			Timeout: b.cfg.Telegram.TypingActionTimeout,
		},
	})
	if err != nil {
		utils.ErrorLog(componentName, "failed to send initial typing action", err,
			utils.KeyAction, "send_initial_typing",
			utils.KeyType, "telegram_api",
			utils.KeyRequestID, chatID)
	}

	ticker := time.NewTicker(b.cfg.Telegram.TypingInterval)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case <-ctx.Done():
					return
				default:
					_, err := bot.SendChatAction(chatID, "typing", &gotgbot.SendChatActionOpts{
						RequestOpts: &gotgbot.RequestOpts{
							Timeout: b.cfg.Telegram.TypingActionTimeout,
						},
					})
					if err != nil {
						utils.ErrorLog(componentName, "failed to send continuous typing action", err,
							utils.KeyAction, "send_continuous_typing",
							utils.KeyType, "telegram_api",
							utils.KeyRequestID, chatID)
					}
				}
			}
		}
	}()

	<-ctx.Done()
	<-done // Wait for goroutine to finish
}

func (b *bot) withRetry(ctx context.Context, fn func(context.Context) error) error {
	return utils.WithRetry(ctx, fn, utils.DefaultRetryConfig())
}
