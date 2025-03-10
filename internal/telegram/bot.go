package telegram

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/scheduler"
	"github.com/edgard/murailobot/internal/utils/logging"
	timeformats "github.com/edgard/murailobot/internal/utils/time"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// New creates a new bot instance.
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

	// Disable telegram bot's debug logging since we handle our own logging
	api.Debug = false

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

// Start begins processing incoming updates.
func (b *Bot) Start() error {
	if err := b.setupCommands(); err != nil {
		return fmt.Errorf("failed to setup commands: %w", err)
	}

	updateConfig := tgbotapi.NewUpdate(defaultUpdateOffset)
	updateConfig.Timeout = defaultUpdateTimeout
	updates := b.api.GetUpdatesChan(updateConfig)

	logging.Info("bot started successfully",
		"admin_id", b.cfg.AdminID)

	b.scheduleDailyAnalysis()

	return b.processUpdates(updates)
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() error {
	b.api.StopReceivingUpdates()
	close(b.running)

	return nil
}

// setupCommands registers bot commands.
func (b *Bot) setupCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "mrl", Description: "Generate AI response"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
		{Command: "mrl_analysis", Description: "Get previous daily user analyses (admin only)"},
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

// processUpdates handles incoming updates.
func (b *Bot) processUpdates(updates tgbotapi.UpdatesChannel) error {
	for {
		select {
		case <-b.running:
			logging.Info("bot stopping due to Stop call")

			return nil
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			if update.Message.IsCommand() {
				b.handleCommand(update)
			} else if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
				if err := b.handleGroupMessage(update.Message); err != nil {
					logging.Error("failed to handle group message",
						"error", err,
						"chat_id", update.Message.Chat.ID)
				}
			}
		}
	}
}

// handleCommand processes bot commands.
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
			logging.Info("unauthorized access",
				"error", err,
				"command", msg.Command(),
				"user_id", msg.From.ID,
				"chat_id", msg.Chat.ID)
		} else {
			logging.Error("command handler error",
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

// handleMessage processes the /mrl command.
func (b *Bot) handleMessage(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	text := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/mrl"))
	if text == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Provide)

		return b.sendMessage(reply)
	}

	logging.Info("processing chat request",
		"user_id", msg.From.ID,
		"message_length", len(text))

	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	response, err := b.ai.Generate(msg.From.ID, text)
	if err != nil {
		logging.Error("failed to generate AI response",
			"error", err,
			"user_id", msg.From.ID,
			"chat_id", msg.Chat.ID)

		errMsg := b.cfg.Messages.GeneralError

		reply := tgbotapi.NewMessage(msg.Chat.ID, errMsg)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			logging.Error("failed to send error message to user",
				"error", sendErr,
				"user_id", msg.From.ID)
		}

		return fmt.Errorf("AI generation failed: %w", err)
	}

	// Always save to chat history for AI context
	if err := b.db.Save(msg.From.ID, text, response); err != nil {
		logging.Warn("failed to save chat history",
			"error", err,
			"user_id", msg.From.ID)
	}

	// If in a group, also save as group messages
	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		if err := b.db.SaveGroupMessage(msg.Chat.ID, msg.Chat.Title, msg.From.ID, msg.Text); err != nil {
			logging.Warn("failed to save group message",
				"error", err,
				"user_id", msg.From.ID,
				"group_id", msg.Chat.ID)
		}

		if err := b.db.SaveGroupMessage(msg.Chat.ID, msg.Chat.Title, b.api.Self.ID, response); err != nil {
			logging.Warn("failed to save bot response in group",
				"error", err,
				"group_id", msg.Chat.ID)
		}
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if err := b.sendMessage(reply); err != nil {
		logging.Error("failed to send AI response",
			"error", err,
			"user_id", msg.From.ID)

		return fmt.Errorf("failed to send AI response: %w", err)
	}

	return nil
}

// handleGroupMessage processes messages from group chats.
func (b *Bot) handleGroupMessage(msg *tgbotapi.Message) error {
	if msg == nil || msg.Text == "" {
		return nil
	}

	groupID := msg.Chat.ID
	groupName := msg.Chat.Title
	userID := msg.From.ID

	if err := b.db.SaveGroupMessage(groupID, groupName, userID, msg.Text); err != nil {
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
}

// handleReset processes the /mrl_reset command.
func (b *Bot) handleReset(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		logging.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "reset_history")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	logging.Info("resetting chat history", "user_id", userID)

	if err := b.db.DeleteAll(); err != nil {
		logging.Error("failed to reset chat history",
			"error", err,
			"user_id", userID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			logging.Error("failed to send error message to user",
				"error", sendErr,
				"user_id", userID)
		}

		return fmt.Errorf("history reset failed: %w", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.HistoryReset)
	if err := b.sendMessage(reply); err != nil {
		logging.Error("failed to send reset confirmation",
			"error", err,
			"user_id", userID)

		return fmt.Errorf("history reset succeeded but failed to confirm: %w", err)
	}

	return nil
}

// scheduleDailyAnalysis sets up the daily user analysis job to run at midnight UTC.
func (b *Bot) scheduleDailyAnalysis() {
	err := scheduler.AddJob(
		"daily-user-analysis",
		"0 0 * * *", // Run at midnight UTC
		func() {
			yesterday := time.Now().Add(-hoursInDay * time.Hour)
			b.generateUserAnalyses(yesterday)
		},
	)
	if err != nil {
		logging.Error("failed to schedule daily analysis",
			"error", err)

		return
	}
}

// generateUserAnalyses analyzes behavior for all active users.
func (b *Bot) generateUserAnalyses(date time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(hoursInDay * time.Hour)

	// Get all messages for the time period
	messages, err := b.db.GetGroupMessagesInTimeRange(start, end)
	if err != nil {
		logging.Error("failed to get group messages",
			"error", err,
			"date", date.Format(timeformats.DateOnly))

		return
	}

	if len(messages) == 0 {
		return
	}

	// Generate analysis for all users
	analyses, err := b.ai.GenerateGroupAnalysis(messages)
	if err != nil {
		logging.Error("failed to generate group analysis",
			"error", err,
			"date", date.Format(timeformats.DateOnly))

		return
	}

	// Save all analyses
	for _, analysis := range analyses {
		if err := b.db.SaveUserAnalysis(analysis); err != nil {
			logging.Error("failed to save user analysis",
				"error", err,
				"user_id", analysis.UserID,
				"date", date.Format(timeformats.DateOnly))
		}
	}
}

// handleUserAnalysis retrieves and sends user analyses for the past week.
func (b *Bot) handleUserAnalysis(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		logging.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "get_user_analysis")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	// Calculate date range for retrieving past daily analyses
	now := time.Now().UTC()
	weekAgo := now.AddDate(0, 0, dailySummaryOffset)
	start := time.Date(weekAgo.Year(), weekAgo.Month(), weekAgo.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)

	// Get previously generated daily analyses
	analyses, err := b.db.GetUserAnalysesInTimeRange(start, end)
	if err != nil {
		logging.Error("failed to get user analyses",
			"error", err,
			"user_id", userID,
			"start", start,
			"end", end)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)

		return b.sendMessage(reply)
	}

	if len(analyses) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "No analyses available for the past week. They are generated automatically at midnight.")

		return b.sendMessage(reply)
	}

	// Format analyses by date and user
	var response strings.Builder

	response.WriteString("👤 *Weekly User Analyses*\n\n")

	currentDate := ""

	// Sort analyses by date for consistent display
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].Date.Before(analyses[j].Date)
	})

	for _, analysis := range analyses {
		date := analysis.Date.Format(timeformats.DateOnly)
		if date != currentDate {
			if currentDate != "" {
				response.WriteString("\n")
			}

			response.WriteString(fmt.Sprintf("*%s*\n", date))
			currentDate = date
		}

		response.WriteString(fmt.Sprintf("\n*User ID:* %d\n", analysis.UserID))
		response.WriteString(fmt.Sprintf("*Communication Style:* %s\n", analysis.CommunicationStyle))
		response.WriteString(fmt.Sprintf("*Personality Traits:* %s\n", analysis.PersonalityTraits))
		response.WriteString(fmt.Sprintf("*Behavioral Patterns:* %s\n", analysis.BehavioralPatterns))
		response.WriteString(fmt.Sprintf("*Word Choice Patterns:* %s\n", analysis.WordChoicePatterns))
		response.WriteString(fmt.Sprintf("*Interaction Habits:* %s\n", analysis.InteractionHabits))
		response.WriteString(fmt.Sprintf("*Unique Quirks:* %s\n", analysis.UniqueQuirks))
		response.WriteString(fmt.Sprintf("*Emotional Triggers:* %s\n", analysis.EmotionalTriggers))
		response.WriteString(fmt.Sprintf("*Messages Analyzed:* %d\n", analysis.MessageCount))
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response.String())
	reply.ParseMode = "Markdown"

	return b.sendMessage(reply)
}

// isUserAuthorized checks if a user is authorized for admin actions.
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
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// StartTyping sends periodic typing indicators until the returned channel is closed.
func (b *Bot) StartTyping(chatID int64) chan struct{} {
	done := make(chan struct{})
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)

	// Send initial typing indicator
	if _, err := b.api.Request(action); err != nil {
		logging.Debug("failed to send typing action",
			"error", err,
			"chat_id", chatID)
	}

	// Keep sending typing indicators until done
	go func() {
		ticker := time.NewTicker(defaultTypingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, err := b.api.Request(action); err != nil {
					logging.Debug("failed to send typing action",
						"error", err,
						"chat_id", chatID)
				}
			}
		}
	}()

	return done
}
