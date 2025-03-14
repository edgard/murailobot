// Package telegram implements the Telegram bot functionality, handling
// commands, messages, and user interactions.
package telegram

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/errs"
	"github.com/edgard/murailobot/internal/logging"
	"github.com/edgard/murailobot/internal/scheduler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// New creates a new bot instance.
func New(cfg *config.Config, database db.Database, aiClient ai.Service, sched scheduler.Scheduler) (*Bot, error) {
	if cfg == nil {
		return nil, errs.NewValidationError("nil config", nil)
	}

	if database == nil {
		return nil, errs.NewValidationError("nil database", nil)
	}

	if aiClient == nil {
		return nil, errs.NewValidationError("nil AI service", nil)
	}

	if sched == nil {
		return nil, errs.NewValidationError("nil scheduler", nil)
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, errs.NewConfigError("failed to create telegram bot", err)
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
			Commands: commands{
				Start:    cfg.TelegramStartCommandDescription,
				Mrl:      cfg.TelegramMrlCommandDescription,
				Reset:    cfg.TelegramResetCommandDescription,
				Analyze:  cfg.TelegramAnalyzeCommandDescription,
				Profiles: cfg.TelegramProfilesCommandDescription,
				EditUser: cfg.TelegramEditUserCommandDescription,
			},
			Messages: messages{
				Welcome:        cfg.TelegramWelcomeMessage,
				Unauthorized:   cfg.TelegramNotAuthorizedMessage,
				Provide:        cfg.TelegramProvideMessage,
				GeneralError:   cfg.TelegramGeneralErrorMessage,
				HistoryReset:   cfg.TelegramHistoryResetMessage,
				Analyzing:      cfg.TelegramAnalyzingMessage,
				NoProfiles:     cfg.TelegramNoProfilesMessage,
				InvalidUserID:  cfg.TelegramInvalidUserIDMessage,
				InvalidField:   cfg.TelegramInvalidFieldMessage,
				UpdateSuccess:  cfg.TelegramUpdateSuccessMessage,
				UserEditUsage:  cfg.TelegramUserEditUsageMessage,
				ProfilesHeader: cfg.TelegramProfilesHeaderMessage,
			},
		},
		running: make(chan struct{}),
	}

	// Set the bot's info in the AI client for special handling in profiles
	if err := aiClient.SetBotInfo(api.Self.ID, api.Self.UserName, api.Self.FirstName); err != nil {
		return nil, errs.NewConfigError("failed to set bot info in AI client", err)
	}

	return bot, nil
}

// Start begins processing incoming updates.
func (b *Bot) Start() error {
	if err := b.setupCommands(); err != nil {
		return errs.NewConfigError("failed to setup commands", err)
	}

	updateConfig := tgbotapi.NewUpdate(defaultUpdateOffset)
	updateConfig.Timeout = defaultUpdateTimeout
	updates := b.api.GetUpdatesChan(updateConfig)

	logging.Info("bot started successfully",
		"bot_username", b.api.Self.UserName,
		"bot_id", b.api.Self.ID,
		"admin_id", b.cfg.AdminID)

	if err := b.scheduleDailyAnalysis(); err != nil {
		return err
	}

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
	if b.api == nil {
		return errs.NewConfigError("nil telegram API client", nil)
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: b.cfg.Commands.Start},
		{Command: "mrl", Description: b.cfg.Commands.Mrl},
		{Command: "mrl_reset", Description: b.cfg.Commands.Reset},
		{Command: "mrl_analyze", Description: b.cfg.Commands.Analyze},
		{Command: "mrl_profiles", Description: b.cfg.Commands.Profiles},
		{Command: "mrl_edit_user", Description: b.cfg.Commands.EditUser},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)

	_, err := b.api.Request(cmdConfig)
	if err != nil {
		return errs.NewAPIError("failed to setup bot commands", err)
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
		case update, ok := <-updates:
			// Check if the channel was closed
			if !ok {
				logging.Info("updates channel closed")

				return nil
			}

			// Skip nil messages
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
	case "mrl_edit_user":
		err = b.handleEditUserCommand(msg)
	}

	if err != nil {
		var unauthorizedErr *errs.UnauthorizedError
		if errors.As(err, &unauthorizedErr) {
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
		return errs.NewValidationError("nil message", nil)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Welcome)

	return b.sendMessage(reply)
}

// handleMessage processes the /mrl command.
func (b *Bot) handleMessage(msg *tgbotapi.Message) error {
	if msg == nil {
		return errs.NewValidationError("nil message", nil)
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
		userProfiles = make(map[int64]*db.UserProfile)
	}

	response, err := b.ai.Generate(msg.From.ID, text, userProfiles)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			logging.Error("failed to send error message", "error", sendErr)
		}

		return errs.NewAPIError("failed to generate AI response", err)
	}

	// Save the interaction
	if err := b.saveInteraction(msg, text, response); err != nil {
		logging.Error("failed to save interaction", "error", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if err := b.sendMessage(reply); err != nil {
		return errs.NewAPIError("failed to send AI response", err)
	}

	return nil
}

// saveInteraction saves both chat history and group messages if applicable.
func (b *Bot) saveInteraction(msg *tgbotapi.Message, userText, botResponse string) error {
	// Save to chat history
	if err := b.db.Save(msg.From.ID, userText, botResponse); err != nil {
		return errs.NewDatabaseError("failed to save chat history", err)
	}

	// If in a group, save both messages
	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		if err := b.db.SaveGroupMessage(msg.Chat.ID, msg.Chat.Title, msg.From.ID, msg.Text); err != nil {
			return errs.NewDatabaseError("failed to save user group message", err)
		}

		if err := b.db.SaveGroupMessage(msg.Chat.ID, msg.Chat.Title, b.api.Self.ID, botResponse); err != nil {
			return errs.NewDatabaseError("failed to save bot group message", err)
		}
	}

	return nil
}

// handleGroupMessage processes messages from group chats.
func (b *Bot) handleGroupMessage(msg *tgbotapi.Message) error {
	if msg == nil || msg.Text == "" {
		return nil
	}

	if err := b.db.SaveGroupMessage(msg.Chat.ID, msg.Chat.Title, msg.From.ID, msg.Text); err != nil {
		return errs.NewDatabaseError("failed to save group message", err)
	}

	return nil
}

// handleReset processes the /mrl_reset command.
func (b *Bot) handleReset(msg *tgbotapi.Message) error {
	if msg == nil {
		return errs.NewValidationError("nil message", nil)
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message", "error", err)
		}

		return errs.NewUnauthorizedError("unauthorized access attempt")
	}

	if err := b.db.DeleteChatHistory(); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if sendErr := b.sendMessage(reply); sendErr != nil {
			logging.Error("failed to send error message", "error", sendErr)
		}

		return errs.NewDatabaseError("failed to reset chat history", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.HistoryReset)
	if err := b.sendMessage(reply); err != nil {
		return errs.NewAPIError("failed to send reset confirmation", err)
	}

	return nil
}

// scheduleDailyAnalysis sets up the daily user analysis job to run at midnight UTC.
func (b *Bot) scheduleDailyAnalysis() error {
	// Schedule the main analysis job
	err := b.scheduler.AddJob(
		"daily-profile-update",
		"0 0 * * *", // Run at midnight UTC
		func() {
			if err := b.generateUserAnalyses(); err != nil {
				logging.Error("daily analysis failed", "error", err)
			}
		},
	)
	if err != nil {
		return errs.NewConfigError("failed to schedule daily analysis", err)
	}

	// Schedule a cleanup job to delete processed messages after a safety period
	// Running 12 hours after the main job to ensure analysis is complete
	err = b.scheduler.AddJob(
		"cleanup-processed-messages",
		"0 12 * * *", // Run at noon UTC (12 hours after main job)
		func() {
			if err := b.cleanupProcessedMessages(time.Now().AddDate(0, 0, -7)); err != nil {
				logging.Error("message cleanup failed", "error", err)
			}
		},
	)
	if err != nil {
		return errs.NewConfigError("failed to schedule message cleanup", err)
	}

	return nil
}

// cleanupProcessedMessages deletes messages that were processed before the cutoff time.
func (b *Bot) cleanupProcessedMessages(cutoffTime time.Time) error {
	if err := b.db.DeleteProcessedGroupMessages(cutoffTime); err != nil {
		return errs.NewDatabaseError("failed to delete processed messages", err)
	}

	logging.Info("successfully cleaned up processed messages",
		"cutoff_time", cutoffTime.Format(time.RFC3339))

	return nil
}

// generateUserAnalyses analyzes all unprocessed messages and updates user profiles.
func (b *Bot) generateUserAnalyses() error {
	// Get only unprocessed messages from the database
	unprocessedMessages, err := b.db.GetUnprocessedGroupMessages()
	if err != nil {
		return errs.NewDatabaseError("failed to get unprocessed group messages", err)
	}

	if len(unprocessedMessages) == 0 {
		logging.Info("no unprocessed messages to analyze")

		// Even if there are no new messages, we should still return any existing profiles
		// This avoids an empty response when calling from handleAnalyzeCommand
		return nil
	}

	logging.Info("starting profile analysis",
		"unprocessed_messages", len(unprocessedMessages))

	// Get existing profiles for context
	existingProfiles, err := b.db.GetAllUserProfiles()
	if err != nil {
		existingProfiles = make(map[int64]*db.UserProfile)
	}

	// Generate/update profiles with all unprocessed messages at once
	updatedProfiles, err := b.ai.GenerateUserProfiles(unprocessedMessages, existingProfiles)
	if err != nil {
		return errs.NewAPIError("failed to generate user profiles", err)
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
			return errs.NewDatabaseError("failed to save user profile", err)
		}
	}

	// Collect IDs of processed messages
	messageIDs := make([]uint, 0, len(unprocessedMessages))
	for _, msg := range unprocessedMessages {
		messageIDs = append(messageIDs, msg.ID)
	}

	// Mark all messages as processed
	if err := b.db.MarkGroupMessagesAsProcessed(messageIDs); err != nil {
		return errs.NewDatabaseError("failed to mark messages as processed", err)
	}

	logging.Info("profile update completed",
		"messages_processed", len(unprocessedMessages),
		"profiles_updated", len(updatedProfiles))

	return nil
}

// handleAnalyzeCommand processes the /mrl_analyze command.
func (b *Bot) handleAnalyzeCommand(msg *tgbotapi.Message) error {
	if msg == nil {
		return errs.NewValidationError("nil message", nil)
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message", "error", err)
		}

		return errs.NewUnauthorizedError("unauthorized access attempt")
	}

	// Send processing message
	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Analyzing)
	if err := b.sendMessage(reply); err != nil {
		return errs.NewAPIError("failed to send processing message", err)
	}

	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	// Run the analysis on all messages
	if err := b.generateUserAnalyses(); err != nil {
		return errs.NewAPIError("failed to analyze user messages", err)
	}

	// Get updated profiles to display
	return b.sendUserProfiles(msg.Chat.ID)
}

// handleProfilesCommand processes the /mrl_profiles command.
func (b *Bot) handleProfilesCommand(msg *tgbotapi.Message) error {
	if msg == nil {
		return errs.NewValidationError("nil message", nil)
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message", "error", err)
		}

		return errs.NewUnauthorizedError("unauthorized access attempt")
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
		reply := tgbotapi.NewMessage(chatID, b.cfg.Messages.NoProfiles)

		return b.sendMessage(reply)
	}

	// Format profiles
	var profilesReport strings.Builder

	profilesReport.WriteString(b.cfg.Messages.ProfilesHeader)

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

// Fields: displaynames, origin, location, age, traits.
func (b *Bot) handleEditUserCommand(msg *tgbotapi.Message) error {
	if msg == nil {
		return errs.NewValidationError("nil message", nil)
	}

	// Check if user is admin
	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply); err != nil {
			logging.Error("failed to send unauthorized message", "error", err)
		}

		return errs.NewUnauthorizedError("unauthorized access attempt")
	}

	// Parse command arguments
	// Format: /mrl_edit_user [user_id] [field] [new_value]
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(msg.Text, "/mrl_edit_user")))

	if len(args) < 3 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.UserEditUsage)

		return b.sendMessage(reply)
	}

	// Parse target user ID
	targetUserID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.InvalidUserID)

		return b.sendMessage(reply)
	}

	// Get the field to edit
	field := strings.ToLower(args[1])

	// Get the new value (everything after the field)
	newValue := strings.Join(args[2:], " ")

	// Get the user profile
	profile, err := b.db.GetUserProfile(targetUserID)
	if err != nil {
		logging.Error("failed to get user profile",
			"error", err,
			"target_user_id", targetUserID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)

		return b.sendMessage(reply)
	}

	// Create profile if it doesn't exist
	if profile == nil {
		profile = &db.UserProfile{
			UserID:      targetUserID,
			LastUpdated: time.Now().UTC(),
		}
	}

	// Update the appropriate field
	var fieldName string

	switch field {
	case "displaynames":
		profile.DisplayNames = newValue
		fieldName = "Display Names"
	case "origin":
		profile.OriginLocation = newValue
		fieldName = "Origin Location"
	case "location":
		profile.CurrentLocation = newValue
		fieldName = "Current Location"
	case "age":
		profile.AgeRange = newValue
		fieldName = "Age Range"
	case "traits":
		profile.Traits = newValue
		fieldName = "Traits"
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.InvalidField)

		return b.sendMessage(reply)
	}

	// Update last updated timestamp
	profile.LastUpdated = time.Now().UTC()

	// Save the updated profile
	if err := b.db.SaveUserProfile(profile); err != nil {
		logging.Error("failed to save user profile",
			"error", err,
			"target_user_id", targetUserID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)

		return b.sendMessage(reply)
	}

	// Send confirmation
	confirmation := fmt.Sprintf(b.cfg.Messages.UpdateSuccess, fieldName, targetUserID, newValue)
	reply := tgbotapi.NewMessage(msg.Chat.ID, confirmation)

	return b.sendMessage(reply)
}

// isUserAuthorized checks if a user is authorized for admin actions.
func (b *Bot) isUserAuthorized(userID int64) bool {
	return userID == b.cfg.AdminID
}

// sendMessage sends a message with retry logic.
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	if b.api == nil {
		return errs.NewConfigError("nil telegram API client", nil)
	}

	_, err := b.api.Send(msg)
	if err != nil {
		return errs.NewAPIError("failed to send message", err)
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
