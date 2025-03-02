package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func New(cfg *config.Config, database db.Database, aiService ai.Service) (*Bot, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	if database == nil {
		return nil, errors.New("database is nil")
	}
	if aiService == nil {
		return nil, errors.New("AI service is nil")
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	bot := &Bot{
		api: api,
		db:  database,
		ai:  aiService,
		cfg: &Config{
			Token:   cfg.TelegramToken,
			AdminID: cfg.TelegramAdminID,
			Messages: Messages{
				Welcome:      cfg.TelegramWelcomeMessage,
				Unauthorized: cfg.TelegramNotAuthMessage,
				Provide:      cfg.TelegramProvideMessage,
				AIError:      cfg.TelegramAIErrorMessage,
				GeneralError: cfg.TelegramGeneralError,
				HistoryReset: cfg.TelegramHistoryReset,
				Timeout:      cfg.TelegramAIErrorMessage,
			},
		},
	}

	return bot, nil
}

func (b *Bot) Start(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(DefaultUpdateOffset)
	updateConfig.Timeout = DefaultUpdateTimeout

	updates := b.api.GetUpdatesChan(updateConfig)

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate AI response"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)
	if _, err := b.api.Request(cmdConfig); err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	slog.Info("bot started successfully",
		"bot_username", b.api.Self.UserName,
		"admin_id", b.cfg.AdminID)

	for {
		select {
		case <-ctx.Done():
			slog.Info("bot stopping due to context cancellation")

			return fmt.Errorf("context canceled: %w", ctx.Err())
		case update := <-updates:
			if update.Message == nil || !update.Message.IsCommand() {
				continue
			}

			errCh := make(chan error, 1)

			go func(ctx context.Context, msg *tgbotapi.Message, errCh chan<- error) {
				var err error
				switch msg.Command() {
				case "start":
					err = b.handleStart(ctx, msg)
				case "mrl":
					err = b.handleMessage(ctx, msg)
				case "mrl_reset":
					err = b.handleReset(ctx, msg)
				}

				if err != nil {
					slog.Error("command handler error",
						"error", err,
						"command", msg.Command(),
						"user_id", msg.From.ID,
						"chat_id", msg.Chat.ID)

					select {
					case errCh <- err:
					default:
					}
				}

				close(errCh)
			}(ctx, update.Message, errCh)

			go func(errCh <-chan error) {
				for err := range errCh {
					if err != nil {
						slog.Error("command handler returned error", "error", err)
					}
				}
			}(errCh)
		}
	}
}

func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()

	return nil
}

func (b *Bot) SendContinuousTyping(ctx context.Context, chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		slog.Error("failed to send initial typing action", "error", err, "chat_id", chatID)
	}

	ticker := time.NewTicker(DefaultTypingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
			if _, err := b.api.Request(action); err != nil {
				slog.Error("failed to send typing action", "error", err, "chat_id", chatID)
			}
		}
	}
}
