package telegram

import (
	"context"
	"encoding/json"
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
		running:     make(chan struct{}),
		analyzer:    make(chan struct{}),
		activeUsers: make(map[int64]string),
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

	// Start the daily analysis goroutine
	go b.runDailyAnalysis()

	return b.processUpdates(updates)
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()
	close(b.running)
	close(b.analyzer)

	return nil
}

// setupCommands registers the bot's command list with Telegram.
func (b *Bot) setupCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate AI response"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
		{Command: "mrl_analysis", Description: "Get weekly user analyses (admin only)"},
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
			if update.Message == nil {
				continue
			}

			if update.Message.IsCommand() {
				b.handleCommand(update)
			} else if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
				if err := b.handleGroupMessage(update.Message); err != nil {
					slog.Error("failed to handle group message",
						"error", err,
						"chat_id", update.Message.Chat.ID)
				}
			}
		}
	}
}

// runDailyAnalysis generates user analyses every day.
func (b *Bot) runDailyAnalysis() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-b.analyzer:
			return

		case <-ticker.C:
			now := time.Now()
			if now.Hour() == 0 { // Run at midnight
				yesterday := now.Add(-24 * time.Hour)
				b.generateUserAnalyses(yesterday)
			}
		}
	}
}

// generateUserAnalyses analyzes behavior for all active users.
func (b *Bot) generateUserAnalyses(date time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(hoursInDay * time.Hour)

	// Get all messages for the time period
	messages := make([]db.GroupMessage, 0)

	for userID := range b.activeUsers {
		userMsgs, err := b.db.GetMessagesByUserInTimeRange(userID, start, end)
		if err != nil {
			slog.Error("failed to get user messages",
				"error", err,
				"user_id", userID,
				"date", date.Format("2006-01-02"))

			continue
		}

		messages = append(messages, userMsgs...)
	}

	if len(messages) == 0 {
		return
	}

	// Generate analysis for each active user using the full context
	for userID, userName := range b.activeUsers {
		analysis, err := b.ai.GenerateUserAnalysis(userID, userName, messages)
		if err != nil {
			slog.Error("failed to generate user analysis",
				"error", err,
				"user_id", userID,
				"user_name", userName,
				"date", date.Format("2006-01-02"))

			continue
		}

		if err := b.db.SaveUserAnalysis(analysis); err != nil {
			slog.Error("failed to save user analysis",
				"error", err,
				"user_id", userID,
				"user_name", userName,
				"date", date.Format("2006-01-02"))
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
	case "mrl_analysis":
		err = b.handleUserAnalysis(msg)
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

// handleGroupMessage processes messages from group chats.
func (b *Bot) handleGroupMessage(msg *tgbotapi.Message) error {
	if msg == nil || msg.Text == "" {
		return nil
	}

	groupID := msg.Chat.ID
	groupName := msg.Chat.Title
	userID := msg.From.ID

	userName := ""
	if msg.From.UserName != "" {
		userName = "@" + msg.From.UserName
	} else if msg.From.FirstName != "" {
		userName = msg.From.FirstName
	}

	b.activeUsers[userID] = userName

	if err := b.db.SaveGroupMessage(groupID, groupName, userID, userName, msg.Text); err != nil {
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
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

// handleUserAnalysis retrieves and sends user analyses for the past week (admin only).
func (b *Bot) handleUserAnalysis(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		slog.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "get_user_analysis")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			slog.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	// Calculate date range for the past week
	now := time.Now().UTC()
	weekAgo := now.AddDate(0, 0, dailySummaryOffset)
	start := time.Date(weekAgo.Year(), weekAgo.Month(), weekAgo.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	analyses, err := b.db.GetUserAnalysesByDateRange(start, end)
	if err != nil {
		slog.Error("failed to get weekly analyses",
			"error", err,
			"user_id", userID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)

		return b.sendMessage(reply)
	}

	if len(analyses) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "No user analyses available for the past week.")

		return b.sendMessage(reply)
	}

	// Format analyses by date and user
	var response strings.Builder

	response.WriteString("👤 *Weekly User Analyses*\n\n")

	currentDate := ""

	for _, analysis := range analyses {
		date := analysis.Date.Format("2006-01-02")
		if date != currentDate {
			if currentDate != "" {
				response.WriteString("\n")
			}

			response.WriteString(fmt.Sprintf("*%s*\n", date))
			currentDate = date
		}

		response.WriteString(fmt.Sprintf("\n*User:* %s\n", analysis.UserName))
		response.WriteString(fmt.Sprintf("*Style:* %s\n", analysis.CommunicationStyle))

		// Parse and format traits with error handling
		var traits []string
		if err := json.Unmarshal([]byte(analysis.PersonalityTraits), &traits); err != nil {
			slog.Error("failed to parse personality traits",
				"error", err,
				"user_id", analysis.UserID)
		} else {
			response.WriteString("*Traits:* " + strings.Join(traits, ", ") + "\n")
		}

		// Parse and format patterns with error handling
		var patterns []string
		if err := json.Unmarshal([]byte(analysis.BehavioralPatterns), &patterns); err != nil {
			slog.Error("failed to parse behavioral patterns",
				"error", err,
				"user_id", analysis.UserID)
		} else {
			response.WriteString("*Patterns:* " + strings.Join(patterns, ", ") + "\n")
		}

		// Parse and format mood information with error handling
		var moodData struct {
			Overall    string   `json:"overall"`
			Variations []string `json:"variations"`
			Triggers   []string `json:"triggers"`
			Patterns   []string `json:"patterns"`
		}

		if err := json.Unmarshal([]byte(analysis.Mood), &moodData); err != nil {
			slog.Error("failed to parse mood data",
				"error", err,
				"user_id", analysis.UserID)
		} else {
			response.WriteString(fmt.Sprintf("*Mood:* %s\n", moodData.Overall))

			if len(moodData.Variations) > 0 {
				response.WriteString("*Mood Variations:* " + strings.Join(moodData.Variations, ", ") + "\n")
			}

			if len(moodData.Patterns) > 0 {
				response.WriteString("*Emotional Patterns:* " + strings.Join(moodData.Patterns, ", ") + "\n")
			}
		}

		// Parse and format quirks with error handling
		var quirks []string
		if err := json.Unmarshal([]byte(analysis.Quirks), &quirks); err != nil {
			slog.Error("failed to parse quirks",
				"error", err,
				"user_id", analysis.UserID)
		} else {
			response.WriteString("*Quirks:* " + strings.Join(quirks, ", ") + "\n")
		}

		response.WriteString(fmt.Sprintf("*Messages Analyzed:* %d\n", analysis.MessageCount))
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response.String())
	reply.ParseMode = "Markdown"

	return b.sendMessage(reply)
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
