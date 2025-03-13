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
func New(cfg *config.Config, database db.Database, aiClient ai.Service, sched scheduler.Scheduler) (*Bot, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	if database == nil {
		return nil, ErrNilDatabase
	}

	if aiClient == nil {
		return nil, ErrNilAIService
	}

	if sched == nil {
		return nil, ErrNilScheduler
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	// Disable telegram bot's debug logging since we handle our own logging
	api.Debug = false

	bot := &Bot{
		api:       api,
		db:        database,
		ai:        aiClient,
		scheduler: sched,
		cfg: &botConfig{
			Token:   cfg.TelegramToken,
			AdminID: cfg.TelegramAdminID,
			Messages: messages{
				Welcome:      cfg.TelegramWelcomeMessage,
				Unauthorized: cfg.TelegramNotAuthorizedMessage,
				Provide:      cfg.TelegramProvideMessage,
				GeneralError: cfg.TelegramGeneralErrorMessage,
				HistoryReset: cfg.TelegramHistoryResetMessage,
			},
		},
		running: make(chan struct{}),
	}

	// Set the bot's info in the AI client for special handling in profiles
	aiClient.SetBotInfo(api.Self.ID, api.Self.UserName, api.Self.FirstName)

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
		"bot_username", b.api.Self.UserName,
		"bot_id", b.api.Self.ID,
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
		{Command: "mrl_analyze", Description: "Analyze user messages and update profiles (admin only)"},
		{Command: "mrl_profiles", Description: "Show user profiles (admin only)"},
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
	case "mrl_analyze":
		err = b.handleAnalyzeCommand(msg)
	case "mrl_profiles":
		err = b.handleProfilesCommand(msg)
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

	// Get all user profiles for better group context
	userProfiles, err := b.db.GetAllUserProfiles()
	if err != nil {
		logging.Warn("failed to get user profiles",
			"error", err,
			"user_id", msg.From.ID)

		userProfiles = make(map[int64]*db.UserProfile)
	}

	// Log some stats about available profiles
	if len(userProfiles) > 0 {
		logging.Debug("providing group context for message generation",
			"user_id", msg.From.ID,
			"total_profiles", len(userProfiles),
			"has_user_profile", userProfiles[msg.From.ID] != nil)
	}

	response, err := b.ai.Generate(msg.From.ID, text, userProfiles)
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

	if err := b.db.DeleteChatHistory(); err != nil {
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
	// Schedule the main analysis job
	err := b.scheduler.AddJob(
		"daily-profile-update",
		"0 0 * * *", // Run at midnight UTC
		func() {
			// Process all messages, not just from a specific time period
			b.generateUserAnalyses()
		},
	)
	if err != nil {
		logging.Error("failed to schedule daily analysis",
			"error", err)
		return
	}

	// Schedule a cleanup job to delete processed messages after a safety period
	// Running 12 hours after the main job to ensure analysis is complete
	err = b.scheduler.AddJob(
		"cleanup-processed-messages",
		"0 12 * * *", // Run at noon UTC (12 hours after main job)
		func() {
			// Delete messages that were processed more than 7 days ago
			// This provides a safety window in case we need to reprocess them
			cutoffTime := time.Now().AddDate(0, 0, -7)
			b.cleanupProcessedMessages(cutoffTime)
		},
	)
	if err != nil {
		logging.Error("failed to schedule message cleanup",
			"error", err)
	}
}

// cleanupProcessedMessages deletes messages that were processed before the cutoff time.
func (b *Bot) cleanupProcessedMessages(cutoffTime time.Time) {
	logging.Info("starting cleanup of processed messages",
		"cutoff_time", cutoffTime.Format(timeformats.FullTimestamp))

	if err := b.db.DeleteProcessedGroupMessages(cutoffTime); err != nil {
		logging.Error("failed to delete processed messages",
			"error", err,
			"cutoff_time", cutoffTime.Format(timeformats.FullTimestamp))
		return
	}

	logging.Info("successfully cleaned up processed messages",
		"cutoff_time", cutoffTime.Format(timeformats.FullTimestamp))
}

// generateUserAnalyses analyzes all unprocessed messages and updates user profiles.
func (b *Bot) generateUserAnalyses() {
	// Get only unprocessed messages from the database
	unprocessedMessages, err := b.db.GetUnprocessedGroupMessages()
	if err != nil {
		logging.Error("failed to get unprocessed group messages",
			"error", err)
		return
	}

	if len(unprocessedMessages) == 0 {
		logging.Info("no unprocessed messages to analyze")
		return
	}

	logging.Info("starting profile analysis",
		"unprocessed_messages", len(unprocessedMessages))

	// Get existing profiles for context
	existingProfiles, err := b.db.GetAllUserProfiles()
	if err != nil {
		logging.Error("failed to get existing profiles",
			"error", err)
		existingProfiles = make(map[int64]*db.UserProfile)
	}

	// Generate/update profiles with all unprocessed messages at once
	updatedProfiles, err := b.ai.GenerateUserProfiles(unprocessedMessages, existingProfiles)
	if err != nil {
		logging.Error("failed to generate user profiles",
			"error", err)
		return
	}

	// Merge with existing profiles to preserve existing data
	for userID, newProfile := range updatedProfiles {
		if existingProfile, exists := existingProfiles[userID]; exists {
			// Keep existing data if new data is empty
			if newProfile.DisplayNames == "" {
				newProfile.DisplayNames = existingProfile.DisplayNames
			}
			if newProfile.OriginLocation == "" {
				newProfile.OriginLocation = existingProfile.OriginLocation
			}
			if newProfile.CurrentLocation == "" {
				newProfile.CurrentLocation = existingProfile.CurrentLocation
			}
			if newProfile.AgeRange == "" {
				newProfile.AgeRange = existingProfile.AgeRange
			}
			if newProfile.Traits == "" {
				newProfile.Traits = existingProfile.Traits
			}
			// Preserve other metadata
			newProfile.ID = existingProfile.ID
			newProfile.CreatedAt = existingProfile.CreatedAt
		}
	}

	// Save updated profiles
	for _, profile := range updatedProfiles {
		if err := b.db.SaveUserProfile(profile); err != nil {
			logging.Error("failed to save user profile",
				"error", err,
				"user_id", profile.UserID)
		}
	}

	// Collect IDs of processed messages
	messageIDs := make([]uint, 0, len(unprocessedMessages))
	for _, msg := range unprocessedMessages {
		messageIDs = append(messageIDs, msg.ID)
	}

	// Mark all messages as processed
	if err := b.db.MarkGroupMessagesAsProcessed(messageIDs); err != nil {
		logging.Error("failed to mark messages as processed",
			"error", err)
	}

	logging.Info("profile update completed",
		"messages_processed", len(unprocessedMessages),
		"profiles_updated", len(updatedProfiles))
}

// handleAnalyzeCommand processes the /mrl_analyze command.
func (b *Bot) handleAnalyzeCommand(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		logging.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "analyze_messages")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	// Send processing message
	reply := tgbotapi.NewMessage(msg.Chat.ID, "Analyzing messages and updating user profiles...")
	if err := b.sendMessage(reply); err != nil {
		return fmt.Errorf("failed to send processing message: %w", err)
	}

	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	// Run the analysis on all messages
	b.generateUserAnalyses()

	// Get updated profiles to display
	return b.sendUserProfiles(msg.Chat.ID)
}

// handleProfilesCommand processes the /mrl_profiles command.
func (b *Bot) handleProfilesCommand(msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		logging.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "view_profiles")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	return b.sendUserProfiles(msg.Chat.ID)
}

// sendUserProfiles formats and sends user profiles.
func (b *Bot) sendUserProfiles(chatID int64) error {
	// Get all profiles
	profiles, err := b.db.GetAllUserProfiles()
	if err != nil {
		logging.Error("failed to get user profiles", "error", err)

		reply := tgbotapi.NewMessage(chatID, b.cfg.Messages.GeneralError)

		return b.sendMessage(reply)
	}

	if len(profiles) == 0 {
		reply := tgbotapi.NewMessage(chatID, "No user profiles available. Run /mrl_analyze to generate profiles.")

		return b.sendMessage(reply)
	}

	// Format profiles
	var profilesReport strings.Builder

	profilesReport.WriteString("👤 *User Profiles*\n\n")

	// Sort users by ID for consistent display
	userIDs := make([]int64, 0, len(profiles))
	for userID := range profiles {
		userIDs = append(userIDs, userID)
	}

	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	for _, userID := range userIDs {
		profile := profiles[userID]
		// Use the standardized formatting method
		profilesReport.WriteString(profile.FormatPipeDelimited() + "\n\n")
	}

	reply := tgbotapi.NewMessage(chatID, profilesReport.String())

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
	stopTyping := make(chan struct{})
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)

	// Send initial typing indicator
	if _, err := b.api.Request(action); err != nil {
		logging.Debug("failed to send typing action",
			"error", err,
			"chat_id", chatID)
	}

	// Keep sending typing indicators until stopTyping
	go func() {
		ticker := time.NewTicker(defaultTypingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopTyping:
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

	return stopTyping
}
