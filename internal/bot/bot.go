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
	slog.Debug("initializing bot")

	if database == nil {
		return nil, errors.New("nil database")
	}

	if aiClient == nil {
		return nil, errors.New("nil AI client")
	}

	if scheduler == nil {
		return nil, errors.New("nil scheduler")
	}

	slog.Debug("connecting to Telegram API")
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

	slog.Debug("configuring bot info in AI client")
	if err := aiClient.SetBotInfo(ai.BotInfo{
		UserID:      api.Self.ID,
		Username:    api.Self.UserName,
		DisplayName: api.Self.FirstName,
	}); err != nil {
		slog.Error("failed to set bot info in AI client", "error", err)
		return nil, err
	}

	slog.Debug("registering command handlers")
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

	slog.Debug("setting up bot commands")
	if err := b.setupCommands(); err != nil {
		slog.Error("failed to setup commands", "error", err)
		return err
	}

	slog.Debug("configuring update channel")
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := b.api.GetUpdatesChan(updateConfig)

	slog.Debug("scheduling maintenance tasks")
	if err := b.scheduleMaintenanceTasks(); err != nil {
		slog.Error("failed to schedule maintenance tasks", "error", err)
		return err
	}
	slog.Info("bot started and processing updates")
	return b.processUpdates(updates, errCh)
}

// Stop gracefully shuts down the bot by stopping the update receiver
// and signaling all goroutines to terminate.
func (b *Bot) Stop() error {
	slog.Info("stopping bot")

	b.api.StopReceivingUpdates()
	close(b.running)

	slog.Debug("bot stopped successfully")
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
	slog.Debug("starting to process message updates")

	// Track received message count for periodic logging
	messageCount := 0
	lastLogTime := time.Now()

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

			messageCount++
			if time.Since(lastLogTime) > 5*time.Minute {
				slog.Info("telegram message processing stats",
					"messages_processed", messageCount,
					"period_minutes", 5)
				messageCount = 0
				lastLogTime = time.Now()
			}

			msg := update.Message
			chatID := msg.Chat.ID
			chatType := "private"
			if msg.Chat.IsGroup() {
				chatType = "group"
			} else if msg.Chat.IsSuperGroup() {
				chatType = "supergroup"
			}

			if msg.IsCommand() {
				command := msg.Command()
				// Only log unusual commands at Debug level
				if command != "start" && command != "mrl_profiles" {
					slog.Debug("processing command",
						"command", command,
						"chat_id", chatID)
				}

				if err := b.handleCommand(update); err != nil {
					// Critical errors go to the error channel
					if isCriticalError(err) {
						slog.Error("critical command error",
							"error", err,
							"command", command,
							"chat_id", chatID)
						errCh <- fmt.Errorf("critical command error: %w", err)
					} else {
						// Non-critical errors just get logged
						slog.Error("command error",
							"error", err,
							"command", command,
							"chat_id", chatID)
					}
				}
			} else if chatType == "group" || chatType == "supergroup" {
				// Handle group messages - only log bot mentions at Debug level
				botMention := "@" + b.api.Self.UserName
				isMention := strings.Contains(msg.Text, botMention)

				if isMention {
					slog.Debug("processing bot mention in group",
						"chat_id", chatID,
						"user_id", msg.From.ID)
				}

				if err := b.handleGroupMessage(msg); err != nil {
					if isCriticalError(err) {
						slog.Error("critical group message error",
							"error", err,
							"chat_id", chatID)
						errCh <- fmt.Errorf("critical error in group %d: %w", chatID, err)
					} else {
						slog.Error("group message error",
							"error", err,
							"chat_id", chatID)
					}
				}
			} else if chatType == "private" {
				// Just log but take no action for direct messages - they're not supported
				slog.Debug("ignored private message",
					"chat_id", chatID,
					"user_id", msg.From.ID)
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
			return fmt.Errorf("unauthorized access for command '%s': %w", cmd, err)
		}
		// Return error with context but don't log redundantly
		return fmt.Errorf("command '%s' handler error: %w", cmd, err)
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
		return fmt.Errorf("failed to delete all records: %w", err)
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
	if err := b.validateMessage(msg); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}

	botMention := "@" + b.api.Self.UserName
	if strings.Contains(msg.Text, botMention) {
		slog.Info("processing bot mention",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID)
		return b.handleMentionMessage(msg)
	}

	// For regular messages, just save and return
	message := &db.Message{
		GroupID:   msg.Chat.ID,
		GroupName: msg.Chat.Title,
		UserID:    msg.From.ID,
		Content:   msg.Text,
		Timestamp: time.Now().UTC(),
	}

	if err := b.db.SaveMessage(b.ctx, message); err != nil {
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
}

func (b *Bot) handleMentionMessage(msg *tgbotapi.Message) error {
	// Start typing indicator to provide user feedback
	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	// Collect context for AI response - consolidated logging
	startTime := time.Now()
	recentLimit := 50

	// Get recent messages
	recentMessages, err := b.db.GetRecentMessages(b.ctx, msg.Chat.ID, recentLimit)
	if err != nil {
		b.sendErrorMessage(msg.Chat.ID)
		return fmt.Errorf("failed to fetch recent messages: %w", err)
	}

	// Get user profiles - failure is non-critical
	userProfiles, err := b.db.GetAllUserProfiles(b.ctx)
	if err != nil {
		slog.Warn("proceeding with empty user profiles",
			"error", err,
			"chat_id", msg.Chat.ID)
		userProfiles = make(map[int64]*db.UserProfile)
	}

	slog.Debug("context collection completed",
		"chat_id", msg.Chat.ID,
		"message_count", len(recentMessages),
		"profile_count", len(userProfiles),
		"duration_ms", time.Since(startTime).Milliseconds())

	// Generate AI response - already logged in AI client, no need to log here

	request := &ai.Request{
		UserID:         msg.From.ID,
		Message:        msg.Text,
		RecentMessages: recentMessages,
		UserProfiles:   userProfiles,
	}

	response, err := b.ai.GenerateResponse(b.ctx, request)
	if err != nil {
		b.sendErrorMessage(msg.Chat.ID)
		return fmt.Errorf("failed to generate AI response: %w", err)
	}

	// AI response generation already logged in AI client

	// Store messages - both operations are non-critical to the main flow
	timestamp := time.Now().UTC()
	messages := []*db.Message{
		{
			GroupID:   msg.Chat.ID,
			GroupName: msg.Chat.Title,
			UserID:    msg.From.ID,
			Content:   msg.Text,
			Timestamp: timestamp,
		},
		{
			GroupID:   msg.Chat.ID,
			GroupName: msg.Chat.Title,
			UserID:    b.api.Self.ID,
			Content:   response,
			Timestamp: timestamp,
		},
	}

	// Store both messages
	for i, message := range messages {
		if err := b.db.SaveMessage(b.ctx, message); err != nil {
			slog.Warn("failed to save message",
				"error", err,
				"chat_id", msg.Chat.ID,
				"is_bot", i == 1)
			// Continue since this is non-critical
		}
	}

	// Send the response
	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if err := b.SendMessage(reply); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (b *Bot) scheduleMaintenanceTasks() error {
	slog.Debug("scheduling daily profile update task", "cron", "0 0 * * *")
	err := b.scheduler.AddJob(
		"daily-profile-update",
		"0 0 * * *",
		func() {
			slog.Debug("starting scheduled daily profile update")
			startTime := time.Now()

			if err := b.processAndUpdateUserProfiles(); err != nil {
				slog.Error("daily profile update failed", "error", err)
			} else {
				duration := time.Since(startTime)
				slog.Info("daily profile update completed",
					"duration_ms", duration.Milliseconds())
			}
		},
	)
	if err != nil {
		slog.Error("failed to schedule daily profile update", "error", err)
		return err
	}

	slog.Debug("scheduling daily messages cleanup task", "cron", "0 12 * * *")
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

	slog.Info("maintenance tasks scheduled successfully")
	return nil
}

func (b *Bot) cleanupProcessedMessages(cutoffTime time.Time) error {
	slog.Debug("starting message cleanup process",
		"cutoff_time", cutoffTime.Format(time.RFC3339))

	// Get all unique group chats that have messages in the database
	groups, err := b.db.GetUniqueGroupChats(b.ctx)
	if err != nil {
		slog.Error("failed to get unique group chats", "error", err)
		return err
	}
	slog.Debug("found groups to process", "group_count", len(groups))

	// Set a reasonable limit for the number of recent messages to preserve per group
	// This balances between maintaining sufficient conversation context and database size
	messagesPerGroup := 1000
	slog.Debug("messages to preserve per group", "limit", messagesPerGroup)

	totalProcessed := 0
	totalPreserved := 0

	// Process each group chat separately to maintain independent conversation histories
	for i, groupID := range groups {
		slog.Debug("processing group",
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
			slog.Debug("cleaned up messages for group",
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
	slog.Debug("starting user profile update process")

	// Retrieve all messages that haven't been processed for user profile analysis yet
	unprocessedMessages, err := b.db.GetUnprocessedMessages(b.ctx)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	// If there are no unprocessed messages, there's nothing to do
	if len(unprocessedMessages) == 0 {
		slog.Debug("no unprocessed messages found, skipping profile update")
		return nil
	}

	slog.Debug("retrieved unprocessed messages", "count", len(unprocessedMessages))

	// Get existing user profiles to provide context for the AI analysis
	existingProfiles, err := b.db.GetAllUserProfiles(b.ctx)
	if err != nil {
		// If we can't get existing profiles, start with an empty map
		slog.Warn("failed to get existing profiles", "error", err)
		existingProfiles = make(map[int64]*db.UserProfile)
	}

	// Use AI to analyze messages and generate updated user profiles
	slog.Debug("generating user profiles with AI",
		"message_count", len(unprocessedMessages),
		"existing_profiles", len(existingProfiles))

	updatedProfiles, err := b.ai.GenerateProfiles(b.ctx, unprocessedMessages, existingProfiles)
	if err != nil {
		return fmt.Errorf("failed to generate profiles with AI: %w", err)
	}

	// Merge new profile data with existing profiles
	for userID, newProfile := range updatedProfiles {
		if existingProfile, exists := existingProfiles[userID]; exists {
			// For each field, keep existing value if the AI didn't provide a new one
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
		}
	}

	// Save all updated profiles to the database
	for userID, profile := range updatedProfiles {
		if err := b.db.SaveUserProfile(b.ctx, profile); err != nil {
			slog.Error("failed to save user profile", "error", err, "user_id", userID)
			return err
		}
	}

	// Mark all processed messages as such to avoid reprocessing them
	messageIDs := make([]uint, 0, len(unprocessedMessages))
	for _, msg := range unprocessedMessages {
		messageIDs = append(messageIDs, msg.ID)
	}

	if err := b.db.MarkMessagesAsProcessed(b.ctx, messageIDs); err != nil {
		return fmt.Errorf("failed to mark messages as processed: %w", err)
	}

	slog.Info("user profile update completed", "profiles_updated", len(updatedProfiles))
	return nil
}

// StartTyping sends periodic typing indicators to a chat until the returned
// channel is closed. This provides visual feedback to users that the bot
// is processing their request, especially during longer operations.
//
// The returned channel should be closed when typing indicators are no longer needed.
func (b *Bot) StartTyping(chatID int64) chan struct{} {
	stopTyping := make(chan struct{})

	// Fast path - if no API client, just return the channel
	if b.api == nil {
		return stopTyping
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)

	// Send initial typing indicator - errors are non-critical
	// Using blank identifier to explicitly ignore errors with typing indicators
	_, _ = b.api.Request(action)

	ctx, cancel := context.WithCancel(b.ctx)

	// Start background goroutine to periodically send typing indicators
	go func() {
		defer cancel()

		// Typing indicators every 4 seconds
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		// Failure count for logging control
		failureCount := 0

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
					// Using blank identifier to explicitly ignore errors
					// We just log occasionally if there are multiple failures
					if _, err := b.api.Request(action); err != nil {
						failureCount++
						if failureCount%3 == 0 {
							slog.Debug("multiple typing action failures",
								"chat_id", chatID,
								"count", failureCount)
						}
					} else {
						failureCount = 0
					}
				}
			}
		}
	}()

	return stopTyping
}

// SendMessage sends a message to a Telegram chat.
// It returns an error if the Telegram API client is nil or if sending fails.
func (b *Bot) SendMessage(msg tgbotapi.MessageConfig) error {
	if b.api == nil {
		return errors.New("nil telegram API client")
	}

	// Only log large messages
	if len(msg.Text) > 500 {
		slog.Debug("sending large message", "chat_id", msg.ChatID, "length", len(msg.Text))
	}

	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message to chat %d: %w", msg.ChatID, err)
	}

	return nil
}

// SendUserProfiles formats and sends all user profiles to the specified chat.
// It retrieves profiles from the database, formats them in a readable way,
// and sends them as a message to the chat.
func (b *Bot) SendUserProfiles(ctx context.Context, chatID int64) error {
	profiles, err := b.db.GetAllUserProfiles(ctx)
	if err != nil {
		b.sendErrorMessage(chatID)
		return fmt.Errorf("failed to get user profiles: %w", err)
	}

	if len(profiles) == 0 {
		reply := tgbotapi.NewMessage(chatID, b.config.BotMsgNoProfiles)
		return b.SendMessage(reply)
	}

	var profilesReport strings.Builder
	profilesReport.WriteString(b.config.BotMsgProfilesHeader)

	// Sort user IDs for consistent display
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

// IsAuthorized checks if a user is authorized for admin actions
// by comparing their ID with the configured admin ID.
func (b *Bot) IsAuthorized(userID int64) bool {
	return userID == b.config.BotAdminID
}
