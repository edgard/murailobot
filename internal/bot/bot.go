// Package bot implements Telegram bot functionality for MurailoBot, handling
// message processing, command execution, and user profile management.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents a Telegram bot instance with all required dependencies
// for handling messages, managing user profiles, and interacting with
// the Telegram API.
type Bot struct {
	api       *tgbotapi.BotAPI
	db        *db.DB
	ai        *ai.Client
	scheduler *utils.Scheduler
	config    *config.Config
	running   chan struct{}
	handlers  map[string]func(*tgbotapi.Message) error
	ctx       context.Context
}

// New creates a new bot instance with the provided dependencies.
// It initializes the Telegram API client, sets up command handlers,
// and configures the bot's information in the AI client.
func New(cfg *config.Config, database *db.DB, aiClient *ai.Client, scheduler *utils.Scheduler) (*Bot, error) {
	slog.Info("initializing bot")

	if database == nil {
		return nil, errors.New("nil database")
	}

	if aiClient == nil {
		return nil, errors.New("nil AI client")
	}

	if scheduler == nil {
		return nil, errors.New("nil scheduler")
	}

	slog.Info("connecting to Telegram API")
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		slog.Error("failed to connect to Telegram API", "error", err)
		return nil, err
	}
	slog.Info("connected to Telegram API",
		"bot_username", api.Self.UserName,
		"bot_id", api.Self.ID)

	api.Debug = false
	ctx := context.Background()

	bot := &Bot{
		api:       api,
		db:        database,
		ai:        aiClient,
		scheduler: scheduler,
		config:    cfg,
		running:   make(chan struct{}),
		ctx:       ctx,
	}

	slog.Info("configuring bot info in AI client")
	if err := aiClient.SetBotInfo(ai.BotInfo{
		UserID:      api.Self.ID,
		Username:    api.Self.UserName,
		DisplayName: api.Self.FirstName,
	}); err != nil {
		slog.Error("failed to set bot info in AI client", "error", err)
		return nil, err
	}

	slog.Info("registering command handlers")
	bot.handlers = map[string]func(*tgbotapi.Message) error{
		"start":         bot.handleStartCommand,
		"mrl_reset":     bot.handleResetCommand,
		"mrl_analyze":   bot.handleAnalyzeCommand,
		"mrl_profiles":  bot.handleProfilesCommand,
		"mrl_edit_user": bot.handleEditUserCommand,
	}
	slog.Info("bot initialization complete")

	return bot, nil
}

// Start begins processing incoming Telegram updates.
// It registers bot commands, configures the update channel,
// schedules maintenance tasks, and starts the message processing loop.
// The errCh parameter allows reporting runtime errors back to the main goroutine.
func (b *Bot) Start(errCh chan<- error) error {
	slog.Info("starting bot")

	slog.Info("setting up bot commands")
	if err := b.setupCommands(); err != nil {
		slog.Error("failed to setup commands", "error", err)
		return err
	}
	slog.Info("bot commands registered successfully")

	slog.Info("configuring update channel")
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := b.api.GetUpdatesChan(updateConfig)
	slog.Info("update channel configured", "timeout", updateConfig.Timeout)

	slog.Info("scheduling maintenance tasks")
	if err := b.scheduleMaintenanceTasks(); err != nil {
		slog.Error("failed to schedule maintenance tasks", "error", err)
		return err
	}
	slog.Info("maintenance tasks scheduled successfully")

	slog.Info("starting to process updates")
	return b.processUpdates(updates, errCh)
}

// Stop gracefully shuts down the bot by stopping the update receiver
// and signaling all goroutines to terminate.
func (b *Bot) Stop() error {
	slog.Info("stopping bot")

	slog.Info("stopping update receiver")
	b.api.StopReceivingUpdates()

	slog.Info("signaling goroutines to terminate")
	close(b.running)

	slog.Info("bot stopped successfully")
	return nil
}

func (b *Bot) setupCommands() error {
	if b.api == nil {
		return errors.New("nil telegram API client")
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: b.config.BotCmdStart},
		{Command: "mrl_reset", Description: b.config.BotCmdReset},
		{Command: "mrl_analyze", Description: b.config.BotCmdAnalyze},
		{Command: "mrl_profiles", Description: b.config.BotCmdProfiles},
		{Command: "mrl_edit_user", Description: b.config.BotCmdEditUser},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)

	_, err := b.api.Request(cmdConfig)

	return err
}

func (b *Bot) processUpdates(updates tgbotapi.UpdatesChannel, errCh chan<- error) error {
	slog.Info("starting to process message updates")

	for {
		select {
		case <-b.running:
			slog.Info("update processing stopped due to bot shutdown")
			return nil
		case update, ok := <-updates:
			if !ok {
				slog.Info("update channel closed")
				return nil
			}

			if update.Message == nil {
				continue
			}

			// Log basic message information
			chatType := "private"
			if update.Message.Chat.IsGroup() {
				chatType = "group"
			} else if update.Message.Chat.IsSuperGroup() {
				chatType = "supergroup"
			}

			slog.Debug("received message",
				"message_id", update.Message.MessageID,
				"chat_id", update.Message.Chat.ID,
				"chat_type", chatType,
				"user_id", update.Message.From.ID,
				"username", update.Message.From.UserName)

			if update.Message.IsCommand() {
				command := update.Message.Command()
				slog.Debug("processing command",
					"command", command,
					"chat_id", update.Message.Chat.ID,
					"user_id", update.Message.From.ID)

				if err := b.handleCommand(update); err != nil {
					// Only send critical errors that should terminate the application
					if isCriticalError(err) {
						slog.Error("critical command error",
							"error", err,
							"command", command,
							"chat_id", update.Message.Chat.ID)
						errCh <- fmt.Errorf("critical command error: %w", err)
					} else {
						slog.Error("command handler error",
							"error", err,
							"command", command,
							"chat_id", update.Message.Chat.ID)
					}
				} else {
					slog.Info("command processed successfully",
						"command", command,
						"chat_id", update.Message.Chat.ID)
				}
			} else if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
				slog.Debug("processing group message",
					"chat_id", update.Message.Chat.ID,
					"chat_title", update.Message.Chat.Title,
					"message_length", len(update.Message.Text))

				if err := b.handleGroupMessage(update.Message); err != nil {
					// Only send critical errors that should terminate the application
					if isCriticalError(err) {
						slog.Error("critical group message error",
							"error", err,
							"chat_id", update.Message.Chat.ID)
						errCh <- fmt.Errorf("critical group message error: %w", err)
					} else {
						slog.Error("failed to handle group message",
							"error", err,
							"chat_id", update.Message.Chat.ID)
					}
				} else {
					slog.Info("group message processed successfully",
						"chat_id", update.Message.Chat.ID)
				}
			} else {
				slog.Info("ignoring non-group message",
					"chat_id", update.Message.Chat.ID,
					"chat_type", chatType)
			}
		}
	}
}

// isCriticalError determines if an error is critical enough to warrant
// terminating the application. This is a simple implementation that
// can be expanded based on specific error types or conditions.
func isCriticalError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific error types that are considered critical
	// For example, database connection failures, API authentication errors, etc.
	if strings.Contains(err.Error(), "database connection lost") {
		return true
	}

	if strings.Contains(err.Error(), "API token revoked") {
		return true
	}

	// Add more conditions as needed

	return false
}

func (b *Bot) handleCommand(update tgbotapi.Update) error {
	msg := update.Message
	cmd := msg.Command()

	handler, exists := b.handlers[cmd]

	var err error

	if exists {
		err = handler(msg)
	}

	if err != nil {
		if strings.Contains(err.Error(), "unauthorized access") {
			slog.Warn("unauthorized access",
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
		return err
	}

	return nil
}

func (b *Bot) sendErrorMessage(chatID int64) {
	reply := tgbotapi.NewMessage(chatID, b.config.BotMsgGeneralError)
	if err := b.SendMessage(reply); err != nil {
		slog.Error("failed to send error message", "error", err)
	}
}

func (b *Bot) checkAuthorization(msg *tgbotapi.Message) error {
	if !b.IsAuthorized(msg.From.ID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgNotAuthorized)
		if err := b.SendMessage(reply); err != nil {
			slog.Error("failed to send unauthorized message", "error", err)
		}

		return errors.New("unauthorized access attempt")
	}

	return nil
}

func (b *Bot) validateMessage(msg *tgbotapi.Message) error {
	if msg == nil || msg.Text == "" {
		return errors.New("empty message")
	}

	if msg.Chat == nil {
		return errors.New("nil chat in message")
	}

	if msg.From == nil {
		return errors.New("nil sender in message")
	}

	if msg.Chat.ID == 0 || msg.From.ID == 0 {
		return errors.New("invalid chat or user ID")
	}

	return nil
}

func (b *Bot) handleStartCommand(msg *tgbotapi.Message) error {
	reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgWelcome)

	return b.SendMessage(reply)
}

func (b *Bot) handleResetCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	if err := b.db.DeleteAll(b.ctx); err != nil {
		b.sendErrorMessage(msg.Chat.ID)

		return err
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgHistoryReset)

	return b.SendMessage(reply)
}

func (b *Bot) handleAnalyzeCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgAnalyzing)
	if err := b.SendMessage(reply); err != nil {
		return err
	}

	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	if err := b.processAndUpdateUserProfiles(); err != nil {
		return err
	}

	return b.SendUserProfiles(b.ctx, msg.Chat.ID)
}

func (b *Bot) handleProfilesCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	return b.SendUserProfiles(b.ctx, msg.Chat.ID)
}

func (b *Bot) handleEditUserCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	args := msg.CommandArguments()
	if args == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Usage: /mrl_edit_user <user_id> <field> <value>")

		return b.SendMessage(reply)
	}

	var userID int64
	var field, value string

	_, err := fmt.Sscanf(args, "%d %s %s", &userID, &field, &value)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Invalid format. Use: /mrl_edit_user <user_id> <field> <value>")

		return b.SendMessage(reply)
	}

	profile, err := b.db.GetUserProfile(b.ctx, userID)
	if err != nil {
		b.sendErrorMessage(msg.Chat.ID)

		return err
	}

	if profile == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("No profile found for user ID %d", userID))

		return b.SendMessage(reply)
	}

	switch strings.ToLower(field) {
	case "displaynames", "display_names":
		profile.DisplayNames = value
	case "originlocation", "origin_location":
		profile.OriginLocation = value
	case "currentlocation", "current_location":
		profile.CurrentLocation = value
	case "agerange", "age_range":
		profile.AgeRange = value
	case "traits":
		profile.Traits = value
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Unknown field: "+field)

		return b.SendMessage(reply)
	}

	if err := b.db.SaveUserProfile(b.ctx, profile); err != nil {
		b.sendErrorMessage(msg.Chat.ID)

		return err
	}

	response := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Updated profile for user ID %d", userID))

	return b.SendMessage(response)
}

func (b *Bot) handleGroupMessage(msg *tgbotapi.Message) error {
	slog.Debug("validating group message",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID)

	if err := b.validateMessage(msg); err != nil {
		slog.Error("invalid message",
			"error", err,
			"chat_id", msg.Chat.ID)
		return err
	}

	botMention := "@" + b.api.Self.UserName
	if strings.Contains(msg.Text, botMention) {
		slog.Info("bot mention detected",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID,
			"username", msg.From.UserName)
		return b.handleMentionMessage(msg)
	} else {
		slog.Debug("saving regular group message",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID)

		message := &db.Message{
			GroupID:   msg.Chat.ID,
			GroupName: msg.Chat.Title,
			UserID:    msg.From.ID,
			Content:   msg.Text,
			Timestamp: time.Now().UTC(),
		}

		err := b.db.SaveMessage(b.ctx, message)
		if err != nil {
			slog.Error("failed to save group message",
				"error", err,
				"chat_id", msg.Chat.ID)
		} else {
			slog.Debug("group message saved successfully",
				"chat_id", msg.Chat.ID)
		}

		return err
	}
}

func (b *Bot) handleMentionMessage(msg *tgbotapi.Message) error {
	slog.Info("handling mention message",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID,
		"username", msg.From.UserName)

	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	recentLimit := 50
	slog.Debug("fetching recent messages",
		"chat_id", msg.Chat.ID,
		"limit", recentLimit)

	startTime := time.Now()
	recentMessages, err := b.db.GetRecentMessages(b.ctx, msg.Chat.ID, recentLimit)
	if err != nil {
		slog.Error("failed to get recent messages",
			"error", err,
			"chat_id", msg.Chat.ID)

		b.sendErrorMessage(msg.Chat.ID)
		return err
	}
	slog.Debug("retrieved recent messages",
		"chat_id", msg.Chat.ID,
		"count", len(recentMessages),
		"duration_ms", time.Since(startTime).Milliseconds())

	slog.Debug("fetching user profiles", "chat_id", msg.Chat.ID)
	userProfiles, err := b.db.GetAllUserProfiles(b.ctx)
	if err != nil {
		slog.Error("failed to get user profiles",
			"error", err,
			"chat_id", msg.Chat.ID)

		userProfiles = make(map[int64]*db.UserProfile)
	}
	slog.Debug("retrieved user profiles",
		"chat_id", msg.Chat.ID,
		"count", len(userProfiles))

	request := &ai.Request{
		UserID:         msg.From.ID,
		Message:        msg.Text,
		RecentMessages: recentMessages,
		UserProfiles:   userProfiles,
	}

	slog.Info("generating AI response",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID,
		"message_length", len(msg.Text))

	aiStartTime := time.Now()
	response, err := b.ai.GenerateResponse(b.ctx, request)
	if err != nil {
		slog.Error("failed to generate AI response",
			"error", err,
			"chat_id", msg.Chat.ID)
		b.sendErrorMessage(msg.Chat.ID)
		return err
	}

	aiDuration := time.Since(aiStartTime)
	slog.Info("AI response generated",
		"chat_id", msg.Chat.ID,
		"duration_ms", aiDuration.Milliseconds(),
		"response_length", len(response))

	slog.Debug("saving user message to database",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID)
	userMessage := &db.Message{
		GroupID:   msg.Chat.ID,
		GroupName: msg.Chat.Title,
		UserID:    msg.From.ID,
		Content:   msg.Text,
		Timestamp: time.Now().UTC(),
	}
	if err := b.db.SaveMessage(b.ctx, userMessage); err != nil {
		slog.Error("failed to save user group message",
			"error", err,
			"chat_id", msg.Chat.ID)
	} else {
		slog.Debug("user message saved successfully",
			"chat_id", msg.Chat.ID)
	}

	slog.Debug("saving bot response to database",
		"chat_id", msg.Chat.ID)
	botMessage := &db.Message{
		GroupID:   msg.Chat.ID,
		GroupName: msg.Chat.Title,
		UserID:    b.api.Self.ID,
		Content:   response,
		Timestamp: time.Now().UTC(),
	}
	if err := b.db.SaveMessage(b.ctx, botMessage); err != nil {
		slog.Error("failed to save bot group message",
			"error", err,
			"chat_id", msg.Chat.ID)
	} else {
		slog.Debug("bot message saved successfully",
			"chat_id", msg.Chat.ID)
	}

	slog.Info("sending response to chat",
		"chat_id", msg.Chat.ID,
		"response_length", len(response))
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)

	err = b.SendMessage(reply)
	if err != nil {
		slog.Error("failed to send message",
			"error", err,
			"chat_id", msg.Chat.ID)
	} else {
		slog.Info("response sent successfully",
			"chat_id", msg.Chat.ID)
	}

	return err
}

func (b *Bot) scheduleMaintenanceTasks() error {
	slog.Info("scheduling daily profile update task", "cron", "0 0 * * *")
	err := b.scheduler.AddJob(
		"daily-profile-update",
		"0 0 * * *",
		func() {
			slog.Info("starting scheduled daily profile update")
			startTime := time.Now()

			if err := b.processAndUpdateUserProfiles(); err != nil {
				slog.Error("daily profile update failed", "error", err)
			} else {
				duration := time.Since(startTime)
				slog.Info("daily profile update completed successfully",
					"duration_ms", duration.Milliseconds())
			}
		},
	)
	if err != nil {
		slog.Error("failed to schedule daily profile update", "error", err)
		return err
	}
	slog.Info("daily profile update scheduled successfully")

	slog.Info("scheduling daily messages cleanup task", "cron", "0 12 * * *")
	err = b.scheduler.AddJob(
		"daily-messages-cleanup",
		"0 12 * * *",
		func() {
			cutoffTime := time.Now().UTC().AddDate(0, 0, -7)
			slog.Info("starting scheduled messages cleanup",
				"cutoff_time", cutoffTime.Format(time.RFC3339))
			startTime := time.Now()

			if err := b.cleanupProcessedMessages(cutoffTime); err != nil {
				slog.Error("daily messages cleanup failed", "error", err)
			} else {
				duration := time.Since(startTime)
				slog.Info("daily messages cleanup completed successfully",
					"duration_ms", duration.Milliseconds())
			}
		},
	)
	if err != nil {
		slog.Error("failed to schedule daily messages cleanup", "error", err)
		return err
	}
	slog.Info("daily messages cleanup scheduled successfully")

	return nil
}

func (b *Bot) cleanupProcessedMessages(cutoffTime time.Time) error {
	slog.Info("starting message cleanup process",
		"cutoff_time", cutoffTime.Format(time.RFC3339))

	// Get all unique group chats that have messages in the database
	slog.Debug("fetching unique group chats")
	groups, err := b.db.GetUniqueGroupChats(b.ctx)
	if err != nil {
		slog.Error("failed to get unique group chats", "error", err)
		return err
	}
	slog.Info("found groups to process", "group_count", len(groups))

	// Set a reasonable limit for the number of recent messages to preserve per group
	// This balances between maintaining sufficient conversation context and database size
	messagesPerGroup := 1000
	slog.Debug("messages to preserve per group", "limit", messagesPerGroup)

	totalProcessed := 0
	totalPreserved := 0

	// Process each group chat separately to maintain independent conversation histories
	for i, groupID := range groups {
		slog.Info("processing group",
			"group_id", groupID,
			"progress", fmt.Sprintf("%d/%d", i+1, len(groups)))

		// Retrieve the most recent messages for this group to preserve them
		// These messages will be kept regardless of their processed status or age
		startTime := time.Now()
		recentMessages, err := b.db.GetRecentMessages(b.ctx, groupID, messagesPerGroup)
		if err != nil {
			slog.Error("failed to get recent messages for group",
				"error", err,
				"group_id", groupID)

			// Continue with other groups even if one fails
			continue
		}

		fetchDuration := time.Since(startTime)
		slog.Debug("retrieved recent messages",
			"group_id", groupID,
			"message_count", len(recentMessages),
			"duration_ms", fetchDuration.Milliseconds())

		totalPreserved += len(recentMessages)

		// Extract the IDs of messages to preserve
		preserveIDs := make([]uint, 0, len(recentMessages))
		for _, msg := range recentMessages {
			preserveIDs = append(preserveIDs, msg.ID)
		}

		// Delete old processed messages while preserving the recent ones
		// This maintains conversation context while keeping the database size manageable
		deleteStartTime := time.Now()
		if err := b.db.DeleteProcessedMessagesExcept(b.ctx, groupID, cutoffTime, preserveIDs); err != nil {
			slog.Error("failed to clean up messages for group",
				"error", err,
				"group_id", groupID)
		} else {
			deleteDuration := time.Since(deleteStartTime)
			slog.Info("cleaned up messages for group",
				"group_id", groupID,
				"duration_ms", deleteDuration.Milliseconds())
			totalProcessed++
		}
	}

	slog.Info("message cleanup completed",
		"groups_processed", totalProcessed,
		"total_groups", len(groups),
		"messages_preserved", totalPreserved)

	return nil
}

func (b *Bot) processAndUpdateUserProfiles() error {
	slog.Info("starting user profile update process")

	// Retrieve all messages that haven't been processed for user profile analysis yet
	slog.Debug("fetching unprocessed messages")
	startTime := time.Now()
	unprocessedMessages, err := b.db.GetUnprocessedMessages(b.ctx)
	if err != nil {
		slog.Error("failed to get unprocessed messages", "error", err)
		return err
	}
	fetchDuration := time.Since(startTime)
	slog.Info("retrieved unprocessed messages",
		"count", len(unprocessedMessages),
		"duration_ms", fetchDuration.Milliseconds())

	// If there are no unprocessed messages, there's nothing to do
	if len(unprocessedMessages) == 0 {
		slog.Info("no unprocessed messages found, skipping profile update")
		return nil
	}

	// Group messages by user for logging purposes
	userMessageCounts := make(map[int64]int)
	for _, msg := range unprocessedMessages {
		userMessageCounts[msg.UserID]++
	}
	slog.Info("message distribution by user", "user_count", len(userMessageCounts))

	// Get existing user profiles to provide context for the AI analysis
	// and to ensure we preserve existing data when merging profiles
	slog.Debug("fetching existing user profiles")
	existingProfiles, err := b.db.GetAllUserProfiles(b.ctx)
	if err != nil {
		// If we can't get existing profiles, start with an empty map
		// This allows the system to continue functioning even if profile retrieval fails
		slog.Warn("failed to get existing profiles, starting with empty map", "error", err)
		existingProfiles = make(map[int64]*db.UserProfile)
	}
	slog.Info("retrieved existing profiles", "count", len(existingProfiles))

	// Use AI to analyze messages and generate updated user profiles
	slog.Info("generating user profiles with AI",
		"message_count", len(unprocessedMessages),
		"existing_profile_count", len(existingProfiles))
	aiStartTime := time.Now()
	updatedProfiles, err := b.ai.GenerateProfiles(b.ctx, unprocessedMessages, existingProfiles)
	if err != nil {
		slog.Error("failed to generate profiles with AI", "error", err)
		return err
	}
	aiDuration := time.Since(aiStartTime)
	slog.Info("AI profile generation completed",
		"duration_ms", aiDuration.Milliseconds(),
		"profiles_generated", len(updatedProfiles))

	// Merge new profile data with existing profiles
	// This ensures we don't lose information when the AI doesn't detect certain attributes
	slog.Debug("merging new profile data with existing profiles")
	updatedCount := 0
	newCount := 0

	for userID, newProfile := range updatedProfiles {
		if existingProfile, exists := existingProfiles[userID]; exists {
			// For each field, keep the existing value if the AI didn't provide a new one
			// This preserves user information across updates
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

			// Preserve database metadata from the existing profile
			newProfile.ID = existingProfile.ID
			newProfile.CreatedAt = existingProfile.CreatedAt
			updatedCount++
		} else {
			newCount++
		}
	}
	slog.Info("profiles prepared for saving",
		"updated_profiles", updatedCount,
		"new_profiles", newCount)

	// Save all updated profiles to the database
	slog.Debug("saving profiles to database")
	saveStartTime := time.Now()
	for userID, profile := range updatedProfiles {
		if err := b.db.SaveUserProfile(b.ctx, profile); err != nil {
			slog.Error("failed to save user profile",
				"error", err,
				"user_id", userID)
			return err
		}
	}
	saveDuration := time.Since(saveStartTime)
	slog.Info("profiles saved to database",
		"count", len(updatedProfiles),
		"duration_ms", saveDuration.Milliseconds())

	// Mark all processed messages as such to avoid reprocessing them
	slog.Debug("marking messages as processed")
	messageIDs := make([]uint, 0, len(unprocessedMessages))
	for _, msg := range unprocessedMessages {
		messageIDs = append(messageIDs, msg.ID)
	}

	markStartTime := time.Now()
	if err := b.db.MarkMessagesAsProcessed(b.ctx, messageIDs); err != nil {
		slog.Error("failed to mark messages as processed", "error", err)
		return err
	}
	markDuration := time.Since(markStartTime)
	slog.Info("messages marked as processed",
		"count", len(messageIDs),
		"duration_ms", markDuration.Milliseconds())

	totalDuration := time.Since(startTime)
	slog.Info("user profile update process completed",
		"total_duration_ms", totalDuration.Milliseconds(),
		"profiles_updated", len(updatedProfiles),
		"messages_processed", len(unprocessedMessages))

	return nil
}

// SendMessage sends a message to a Telegram chat.
// It returns an error if the Telegram API client is nil or if sending fails.
func (b *Bot) SendMessage(msg tgbotapi.MessageConfig) error {
	if b.api == nil {
		slog.Error("cannot send message: nil telegram API client")
		return errors.New("nil telegram API client")
	}

	slog.Debug("sending message to chat",
		"chat_id", msg.ChatID,
		"message_length", len(msg.Text))

	startTime := time.Now()
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		slog.Error("failed to send message",
			"error", err,
			"chat_id", msg.ChatID)
		return err
	}

	duration := time.Since(startTime)
	slog.Debug("message sent successfully",
		"chat_id", msg.ChatID,
		"message_id", sentMsg.MessageID,
		"duration_ms", duration.Milliseconds())

	return nil
}

// StartTyping sends periodic typing indicators to a chat until the returned
// channel is closed. This provides visual feedback to users that the bot
// is processing their request, especially during longer operations.
//
// The returned channel should be closed when typing indicators are no longer needed.
func (b *Bot) StartTyping(chatID int64) chan struct{} {
	stopTyping := make(chan struct{})

	if b.api == nil {
		slog.Error("nil telegram API client in startTyping")
		return stopTyping
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)

	if _, err := b.api.Request(action); err != nil {
		slog.Debug("failed to send typing action",
			"error", err,
			"chat_id", chatID)
	}

	ctx, cancel := context.WithCancel(b.ctx)

	go func() {
		defer cancel()

		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopTyping:
				return
			case <-b.running:
				return
			case <-ticker.C:
				select {
				case <-ctx.Done():
					return
				default:
					_, err := b.api.Request(action)
					if err != nil {
						slog.Debug("failed to send typing action",
							"error", err,
							"chat_id", chatID)
						time.Sleep(500 * time.Millisecond)
					}
				}
			}
		}
	}()

	return stopTyping
}

// IsAuthorized checks if a user is authorized for admin actions
// by comparing their ID with the configured admin ID.
func (b *Bot) IsAuthorized(userID int64) bool {
	return userID == b.config.BotAdminID
}

// SendUserProfiles formats and sends all user profiles to the specified chat.
// It retrieves profiles from the database, formats them in a readable way,
// and sends them as a message to the chat.
func (b *Bot) SendUserProfiles(ctx context.Context, chatID int64) error {
	profiles, err := b.db.GetAllUserProfiles(ctx)
	if err != nil {
		slog.Error("failed to get user profiles", "error", err)
		b.sendErrorMessage(chatID)

		return err
	}

	if len(profiles) == 0 {
		reply := tgbotapi.NewMessage(chatID, b.config.BotMsgNoProfiles)

		return b.SendMessage(reply)
	}

	var profilesReport strings.Builder
	profilesReport.WriteString(b.config.BotMsgProfilesHeader)

	userIDs := make([]int64, 0, len(profiles))
	for userID := range profiles {
		userIDs = append(userIDs, userID)
	}

	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	for _, userID := range userIDs {
		profile := profiles[userID]
		profilesReport.WriteString(profile.FormatPipeDelimited() + "\n\n")
	}

	reply := tgbotapi.NewMessage(chatID, profilesReport.String())

	return b.SendMessage(reply)
}
