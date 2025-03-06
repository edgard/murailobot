package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// New creates a Telegram bot with the provided configuration and dependencies.
func New(cfg *config.Config, database db.Database, aiClient ai.Service) (*Bot, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	if database == nil {
		return nil, ErrNilDatabase
	}

	if aiClient == nil {
		return nil, ErrNilAIService
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	bot := &Bot{
		api: api,
		db:  database,
		ai:  aiClient,
		cfg: &botConfig{
			Token:   cfg.TelegramToken,
			AdminID: cfg.TelegramAdminID,
			Messages: messages{
				Welcome:      cfg.TelegramWelcomeMessage,
				Unauthorized: cfg.TelegramNotAuthorizedMessage,
				Provide:      cfg.TelegramProvideMessage,
				GeneralError: cfg.TelegramGeneralErrorMessage,
				HistoryReset: cfg.TelegramHistoryResetMessage,
				Timeout:      cfg.TelegramTimeoutMessage,
			},
		},
		running: make(chan struct{}),
	}

	return bot, nil
}

// Start begins processing incoming updates and handling commands.
func (b *Bot) Start() error {
	if err := b.setupCommands(); err != nil {
		return fmt.Errorf("failed to setup commands: %w", err)
	}

	updateConfig := tgbotapi.NewUpdate(defaultUpdateOffset)
	updateConfig.Timeout = defaultUpdateTimeout
	updates := b.api.GetUpdatesChan(updateConfig)

	slog.Info("bot started successfully",
		"bot_username", b.api.Self.UserName,
		"admin_id", b.cfg.AdminID)

	return b.processUpdates(updates)
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()
	close(b.running)

	return nil
}

// setupCommands registers the bot's command list with Telegram.
func (b *Bot) setupCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate AI response"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)

	err := retry.Do(
		func() error {
			_, err := b.api.Request(cmdConfig)
			if err != nil {
				return fmt.Errorf("telegram API request failed: %w", err)
			}

			return nil
		},
		retry.Attempts(defaultRetryAttempts),
		retry.Delay(defaultRetryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return fmt.Errorf("failed to setup bot commands: %w", err)
	}

	return nil
}

// processUpdates handles incoming Telegram updates.
func (b *Bot) processUpdates(updates tgbotapi.UpdatesChannel) error {
	for {
		select {
		case <-b.running:
			slog.Info("bot stopping due to Stop call")

			return nil

		case update := <-updates:
			if update.Message == nil || !update.Message.IsCommand() {
				continue
			}

			b.handleCommand(update)
		}
	}
}

// handleCommand routes commands to their handlers.
func (b *Bot) handleCommand(update tgbotapi.Update) {
	msg := update.Message
	cmd := msg.Command()

	var err error

	switch cmd {
	case "start":
		err = b.handleStart(msg)
	case "mrl":
		err = b.handleMessage(msg)
	case "mrl_reset":
		err = b.handleReset(msg)
	}

	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			slog.Info("unauthorized access",
				"error", err,
				"command", msg.Command(),
				"user_id", msg.From.ID,
				"chat_id", msg.Chat.ID)
		} else {
			slog.Error("command handler error",
				"error", err,
				"command", msg.Command(),
				"user_id", msg.From.ID,
				"chat_id", msg.Chat.ID)
		}
	}
}

// handleStart processes the /start command.
func (b *Bot) handleStart(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Welcome)

	return b.sendMessage(reply)
}

// handleMessage processes the /mrl command, generating AI responses.
func (b *Bot) handleMessage(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	text := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/mrl"))
	if text == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Provide)

		return b.sendMessage(reply)
	}

	userName := ""
	if msg.From.UserName != "" {
		userName = "@" + msg.From.UserName
	} else if msg.From.FirstName != "" {
		userName = msg.From.FirstName
	}

	usernameForLog := "unknown"
	if userName != "" {
		usernameForLog = userName
	}

	slog.Info("processing chat request",
		"user_id", msg.From.ID,
		"username", usernameForLog,
		"message_length", len(text))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.StartTyping(ctx, msg.Chat.ID)

	response, err := b.ai.Generate(msg.From.ID, userName, text)
	if err != nil {
		slog.Error("failed to generate AI response",
			"error", err,
			"user_id", msg.From.ID,
			"chat_id", msg.Chat.ID)

		errMsg := b.cfg.Messages.GeneralError
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg = b.cfg.Messages.Timeout
		}

		reply := tgbotapi.NewMessage(msg.Chat.ID, errMsg)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			slog.Error("failed to send error message to user",
				"error", sendErr,
				"user_id", msg.From.ID)
		}

		return fmt.Errorf("AI generation failed: %w", err)
	}

	if err := b.db.Save(msg.From.ID, userName, text, response); err != nil {
		slog.Warn("failed to save chat history",
			"error", err,
			"user_id", msg.From.ID)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if err := b.sendMessage(reply); err != nil {
		slog.Error("failed to send AI response",
			"error", err,
			"user_id", msg.From.ID)

		return fmt.Errorf("failed to send AI response: %w", err)
	}

	return nil
}

// handleReset processes the /mrl_reset command (admin only).
func (b *Bot) handleReset(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		slog.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "reset_history")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			slog.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	slog.Info("resetting chat history", "user_id", userID)

	if err := b.db.DeleteAll(); err != nil {
		slog.Error("failed to reset chat history",
			"error", err,
			"user_id", userID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			slog.Error("failed to send error message to user",
				"error", sendErr,
				"user_id", userID)
		}

		return fmt.Errorf("history reset failed: %w", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.HistoryReset)
	if err := b.sendMessage(reply); err != nil {
		slog.Error("failed to send reset confirmation",
			"error", err,
			"user_id", userID)

		return fmt.Errorf("history reset succeeded but failed to confirm: %w", err)
	}

	return nil
}

// StartTyping sends periodic typing indicators until the context is canceled.
func (b *Bot) StartTyping(ctx context.Context, chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		slog.Debug("failed to send typing action",
			"error", err,
			"chat_id", chatID)
	}

	go func() {
		ticker := time.NewTicker(defaultTypingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := b.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
					slog.Debug("failed to send typing action",
						"error", err,
						"chat_id", chatID)
				}
			}
		}
	}()
}

// isUserAuthorized checks if a user has admin privileges.
func (b *Bot) isUserAuthorized(userID int64) bool {
	return userID == b.cfg.AdminID
}

// getMessageType determines the message category for logging.
func (b *Bot) getMessageType(msgText string) string {
	switch {
	case strings.Contains(msgText, b.cfg.Messages.Welcome):
		return "welcome message"
	case strings.Contains(msgText, b.cfg.Messages.Provide):
		return "prompt message"
	case strings.Contains(msgText, b.cfg.Messages.GeneralError):
		return "error message"
	case strings.Contains(msgText, b.cfg.Messages.Timeout):
		return "timeout message"
	case strings.Contains(msgText, b.cfg.Messages.Unauthorized):
		return "unauthorized message"
	case strings.Contains(msgText, b.cfg.Messages.HistoryReset):
		return "history reset confirmation"
	default:
		return "message"
	}
}

// sendMessage sends a message with retry logic.
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	err := retry.Do(
		func() error {
			_, err := b.api.Send(msg)
			if err != nil {
				return fmt.Errorf("telegram API send failed: %w", err)
			}

			return nil
		},
		retry.Attempts(defaultRetryAttempts),
		retry.Delay(defaultRetryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		messageType := b.getMessageType(msg.Text)

		return fmt.Errorf("failed to send %s: %w", messageType, err)
	}

	return nil
}
