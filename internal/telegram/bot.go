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
// and dependencies. Returns an error if any required dependency is nil or
// initialization fails.
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
// or an error occurs. Supports commands: /start, /mrl, and /mrl_reset.
func (b *Bot) Start(parentCtx context.Context) error {
	if err := b.setupCommands(parentCtx); err != nil {
		return err
	}

	updateConfig := tgbotapi.NewUpdate(defaultUpdateOffset)
	updateConfig.Timeout = defaultUpdateTimeout
	updates := b.api.GetUpdatesChan(updateConfig)

	slog.Info("bot started successfully",
		"bot_username", b.api.Self.UserName,
		"admin_id", b.cfg.AdminID)

	return b.processUpdates(parentCtx, updates)
}

// setupCommands registers the bot commands with Telegram.
func (b *Bot) setupCommands(ctx context.Context) error {
	var err error

	cmdCtx, cmdCancel := context.WithTimeout(ctx, apiOperationTimeout)
	done := make(chan struct{})

	defer cmdCancel()

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate OpenAI response"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)

	go func() {
		_, err = b.api.Request(cmdConfig)
		done <- struct{}{}
	}()

	select {
	case <-cmdCtx.Done():
		return fmt.Errorf("timeout or cancellation while setting bot commands: %w", cmdCtx.Err())
	case <-done:
		close(done)

		if err != nil {
			return fmt.Errorf("failed to set bot commands: %w", err)
		}
	}

	return nil
}

// processUpdates handles the stream of incoming updates from Telegram.
func (b *Bot) processUpdates(ctx context.Context, updates tgbotapi.UpdatesChannel) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("bot stopping due to context cancellation")

			return fmt.Errorf("context canceled: %w", ctx.Err())

		case update := <-updates:
			if update.Message == nil || !update.Message.IsCommand() {
				continue
			}

			b.handleCommand(ctx, update)
		}
	}
}

// handleCommand processes a command from a Telegram update.
func (b *Bot) handleCommand(parentCtx context.Context, update tgbotapi.Update) {
	errCh := make(chan error, 1)

	// Create a request-scoped context with timeout for each command
	go func(update tgbotapi.Update, errCh chan<- error) {
		reqCtx, reqCancel := context.WithTimeout(parentCtx, apiOperationTimeout)
		defer reqCancel()

		msg := update.Message
		cmd := msg.Command()

		var err error

		switch cmd {
		case "start":
			err = b.handleStart(reqCtx, msg)
		case "mrl":
			err = b.handleMessage(reqCtx, msg)
		case "mrl_reset":
			err = b.handleReset(reqCtx, msg)
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
	}(update, errCh)

	go func(errCh <-chan error) {
		for err := range errCh {
			if err != nil {
				slog.Error("command handler returned error", "error", err)
			}
		}
	}(errCh)
}

// Stop gracefully shuts down the bot by stopping the update receiver.
// Returns an error if stopping the bot fails or the context is cancelled.
func (b *Bot) Stop(parentCtx context.Context) error {
	select {
	case <-parentCtx.Done():
		return fmt.Errorf("context canceled before stopping bot: %w", parentCtx.Err())
	default:
	}

	stopCtx, stopCancel := context.WithTimeout(parentCtx, apiOperationTimeout)
	defer stopCancel()

	done := make(chan struct{})

	var stopErr error

	go func() {
		b.api.StopReceivingUpdates()
		close(done)
	}()

	select {
	case <-stopCtx.Done():
		return fmt.Errorf("timeout or cancellation while stopping bot: %w", stopCtx.Err())
	case <-done:
		return stopErr
	}
}

// SendContinuousTyping sends periodic typing indicators to provide visual
// feedback during long-running operations like AI response generation.
// Runs until the context is cancelled.
func (b *Bot) SendContinuousTyping(parentCtx context.Context, chatID int64) {
	typingCtx, typingCancel := context.WithTimeout(parentCtx, apiOperationTimeout)
	defer typingCancel()

	select {
	case <-typingCtx.Done():
		slog.Error("context canceled before sending initial typing action",
			"error", typingCtx.Err(),
			"chat_id", chatID)

		return
	default:
		action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
		if _, err := b.api.Request(action); err != nil {
			slog.Error("failed to send initial typing action", "error", err, "chat_id", chatID)
		}
	}

	ticker := time.NewTicker(defaultTypingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-parentCtx.Done():
			return
		case <-ticker.C:
			// Create a new timeout context for each typing action
			typingCtx, typingCancel := context.WithTimeout(parentCtx, apiOperationTimeout)
			action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)

			select {
			case <-typingCtx.Done():
				typingCancel()

				return
			default:
				if _, err := b.api.Request(action); err != nil {
					slog.Error("failed to send typing action", "error", err, "chat_id", chatID)
				}

				typingCancel()
			}
		}
	}
}
