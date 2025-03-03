package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// New creates a new Telegram bot instance with the provided configuration
// and dependencies. It validates the configuration and initializes the
// Telegram Bot API client.
//
// Parameters:
//   - cfg: Configuration containing bot token and settings
//   - database: Database interface for conversation history
//   - openAIService: OpenAI service for generating responses
//
// Returns an error if any required dependency is nil or initialization fails.
func New(cfg *config.Config, database db.Database, openAIService openai.Service) (*Bot, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	db := database
	if db == nil {
		return nil, ErrNilDatabase
	}

	openAI := openAIService
	if openAI == nil {
		return nil, ErrNilOpenAIService
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	bot := &Bot{
		api:    api,
		db:     database,
		openAI: openAIService,
		cfg: &Config{
			Token:   cfg.TelegramToken,
			AdminID: cfg.TelegramAdminID,
			Messages: Messages{
				Welcome:      cfg.TelegramWelcomeMessage,
				Unauthorized: cfg.TelegramNotAuthorizedMessage,
				Provide:      cfg.TelegramProvideMessage,
				OpenAIError:  cfg.TelegramAIErrorMessage,
				GeneralError: cfg.TelegramGeneralErrorMessage,
				HistoryReset: cfg.TelegramHistoryResetMessage,
				Timeout:      cfg.TelegramAIErrorMessage,
			},
		},
	}

	return bot, nil
}

// Start begins the bot's operation, setting up command handlers and
// processing incoming updates. It runs until the context is cancelled
// or an error occurs.
//
// The bot supports the following commands:
//   - /start: Initiates conversation with welcome message
//   - /mrl: Generates AI response to user message
//   - /mrl_reset: Clears chat history (admin only)
//
// The function handles updates asynchronously and manages error reporting
// through channels.
func (b *Bot) Start(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(defaultUpdateOffset)
	updateConfig.Timeout = defaultUpdateTimeout

	updates := b.api.GetUpdatesChan(updateConfig)

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate OpenAI response"},
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
				cmd := msg.Command()

				var err error

				switch cmd {
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

// Stop gracefully shuts down the bot by stopping the update receiver.
// This method should be called when terminating the bot's operation.
func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()

	return nil
}

// SendContinuousTyping sends periodic typing indicators to indicate the bot
// is processing a request. This provides visual feedback during long-running
// operations like AI response generation.
//
// The function runs until the context is cancelled, sending typing indicators
// at defaultTypingInterval intervals.
func (b *Bot) SendContinuousTyping(ctx context.Context, chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		slog.Error("failed to send initial typing action", "error", err, "chat_id", chatID)
	}

	ticker := time.NewTicker(defaultTypingInterval)
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
