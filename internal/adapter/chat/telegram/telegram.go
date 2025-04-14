// Package telegram provides a Telegram implementation of the chat service port.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/common/util"
	"github.com/edgard/murailobot/internal/domain/model"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/chat"
	"github.com/edgard/murailobot/internal/port/scheduler"
	"github.com/edgard/murailobot/internal/port/store"
)

// telegramChat implements the chat.Service interface using Telegram's API
type telegramChat struct {
	api       *tgbotapi.BotAPI
	store     store.Store
	ai        ai.Service
	scheduler scheduler.Service
	config    *config.Config
	running   chan struct{}
	handlers  map[string]func(*tgbotapi.Message) error
	ctx       context.Context
	logger    *zap.Logger
}

// NewChatService creates a new Telegram chat service with the provided dependencies.
func NewChatService(
	cfg *config.Config,
	store store.Store,
	aiService ai.Service,
	scheduler scheduler.Service,
	logger *zap.Logger,
) (chat.Service, error) {
	if store == nil {
		return nil, errors.New("nil store")
	}

	if aiService == nil {
		return nil, errors.New("nil AI service")
	}

	if scheduler == nil {
		return nil, errors.New("nil scheduler")
	}

	logger.Debug("connecting to Telegram API")

	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		logger.Error("failed to connect to Telegram API", zap.Error(err))
		return nil, err
	}

	logger.Info("connected to Telegram API",
		zap.String("bot_username", api.Self.UserName),
		zap.Int64("bot_id", api.Self.ID))

	api.Debug = false
	ctx := context.Background()

	b := &telegramChat{
		api:       api,
		store:     store,
		ai:        aiService,
		scheduler: scheduler,
		config:    cfg,
		running:   make(chan struct{}),
		ctx:       ctx,
		logger:    logger,
	}

	logger.Debug("configuring bot info in AI service")

	if err := aiService.SetBotInfo(ai.BotInfo{
		UserID:      api.Self.ID,
		Username:    api.Self.UserName,
		DisplayName: api.Self.FirstName,
	}); err != nil {
		logger.Error("failed to set bot info in AI service", zap.Error(err))
		return nil, err
	}

	logger.Debug("registering command handlers")

	b.handlers = map[string]func(*tgbotapi.Message) error{
		"start":         b.handleStartCommand,
		"mrl_reset":     b.handleResetCommand,
		"mrl_analyze":   b.handleAnalyzeCommand,
		"mrl_profiles":  b.handleProfilesCommand,
		"mrl_edit_user": b.handleEditUserCommand,
	}

	logger.Info("chat service initialization complete")

	return b, nil
}

// Start begins processing incoming Telegram updates
func (b *telegramChat) Start(errCh chan<- error) error {
	b.logger.Info("starting chat service")

	b.logger.Debug("setting up bot commands")

	if err := b.setupCommands(); err != nil {
		b.logger.Error("failed to setup commands", zap.Error(err))
		return err
	}

	b.logger.Debug("configuring update channel")

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := b.api.GetUpdatesChan(updateConfig)

	b.logger.Debug("scheduling maintenance tasks")

	if err := b.scheduleMaintenanceTasks(); err != nil {
		b.logger.Error("failed to schedule maintenance tasks", zap.Error(err))
		return err
	}

	b.logger.Info("chat service started and processing updates")

	return b.processUpdates(updates, errCh)
}

// Stop gracefully shuts down the chat service
func (b *telegramChat) Stop() error {
	b.logger.Info("stopping chat service")

	b.api.StopReceivingUpdates()
	close(b.running)

	b.logger.Debug("chat service stopped successfully")

	return nil
}

// IsAuthorized checks if a user is authorized for admin actions
func (b *telegramChat) IsAuthorized(userID int64) bool {
	return userID == b.config.BotAdminID
}

// SendUserProfiles formats and sends all user profiles to the specified chat
func (b *telegramChat) SendUserProfiles(ctx context.Context, chatID int64) error {
	profiles, err := b.store.GetAllUserProfiles(ctx)
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

// Helper methods

func (b *telegramChat) setupCommands() error {
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

func (b *telegramChat) processUpdates(updates tgbotapi.UpdatesChannel, errCh chan<- error) error {
	b.logger.Debug("starting to process message updates")

	// Track received message count for periodic logging
	messageCount := 0
	lastLogTime := time.Now()

	for {
		select {
		case <-b.running:
			b.logger.Info("update processing stopped due to chat service shutdown")
			return nil
		case update, ok := <-updates:
			if !ok {
				b.logger.Info("update channel closed")
				return nil
			}

			if update.Message == nil {
				continue
			}

			messageCount++
			if time.Since(lastLogTime) > 5*time.Minute {
				b.logger.Info("telegram message processing stats",
					zap.Int("messages_processed", messageCount),
					zap.Int("period_minutes", 5))

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
					b.logger.Debug("processing command",
						zap.String("command", command),
						zap.Int64("chat_id", chatID))
				}

				if err := b.handleCommand(update); err != nil {
					// Critical errors go to the error channel
					if isCriticalError(err) {
						b.logger.Error("critical command error",
							zap.Error(err),
							zap.String("command", command),
							zap.Int64("chat_id", chatID))
						errCh <- fmt.Errorf("critical command error: %w", err)
					} else {
						// Non-critical errors just get logged
						b.logger.Error("command error",
							zap.Error(err),
							zap.String("command", command),
							zap.Int64("chat_id", chatID))
					}
				}
			} else if chatType == "group" || chatType == "supergroup" {
				// Handle group messages - only log bot mentions at Debug level
				botMention := "@" + b.api.Self.UserName
				isMention := strings.Contains(msg.Text, botMention)

				if isMention {
					b.logger.Debug("processing bot mention in group",
						zap.Int64("chat_id", chatID),
						zap.Int64("user_id", msg.From.ID))
				}

				if err := b.handleGroupMessage(msg); err != nil {
					if isCriticalError(err) {
						b.logger.Error("critical group message error",
							zap.Error(err),
							zap.Int64("chat_id", chatID))
						errCh <- fmt.Errorf("critical error in group %d: %w", chatID, err)
					} else {
						b.logger.Error("group message error",
							zap.Error(err),
							zap.Int64("chat_id", chatID))
					}
				}
			} else if chatType == "private" {
				// Just log but take no action for direct messages - they're not supported
				b.logger.Debug("ignored private message",
					zap.Int64("chat_id", chatID),
					zap.Int64("user_id", msg.From.ID))
			}
		}
	}
}

// Command handlers

func (b *telegramChat) handleCommand(update tgbotapi.Update) error {
	msg := update.Message
	cmd := msg.Command()

	handler, exists := b.handlers[cmd]

	var err error
	if exists {
		err = handler(msg)
	}

	if err != nil {
		if strings.Contains(err.Error(), "unauthorized access") {
			b.logger.Warn("unauthorized access",
				zap.Error(err),
				zap.String("command", msg.Command()),
				zap.Int64("user_id", msg.From.ID),
				zap.Int64("chat_id", msg.Chat.ID))

			return fmt.Errorf("unauthorized access for command '%s': %w", cmd, err)
		}
		// Return error with context but don't log redundantly
		return fmt.Errorf("command '%s' handler error: %w", cmd, err)
	}

	return nil
}

func (b *telegramChat) handleStartCommand(msg *tgbotapi.Message) error {
	reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgWelcome)
	return b.SendMessage(reply)
}

func (b *telegramChat) handleResetCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	if err := b.store.DeleteAll(b.ctx); err != nil {
		b.sendErrorMessage(msg.Chat.ID)
		return fmt.Errorf("failed to delete all records: %w", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgHistoryReset)
	return b.SendMessage(reply)
}

func (b *telegramChat) handleAnalyzeCommand(msg *tgbotapi.Message) error {
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

func (b *telegramChat) handleProfilesCommand(msg *tgbotapi.Message) error {
	if err := b.checkAuthorization(msg); err != nil {
		return err
	}

	return b.SendUserProfiles(b.ctx, msg.Chat.ID)
}

func (b *telegramChat) handleEditUserCommand(msg *tgbotapi.Message) error {
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

	profile, err := b.store.GetUserProfile(b.ctx, userID)
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

	if err := b.store.SaveUserProfile(b.ctx, profile); err != nil {
		b.sendErrorMessage(msg.Chat.ID)
		return err
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Updated profile for user ID %d", userID))
	return b.SendMessage(reply)
}

func (b *telegramChat) handleGroupMessage(msg *tgbotapi.Message) error {
	if err := b.validateMessage(msg); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}

	botMention := "@" + b.api.Self.UserName
	if strings.Contains(msg.Text, botMention) {
		b.logger.Info("processing bot mention",
			zap.Int64("chat_id", msg.Chat.ID),
			zap.Int64("user_id", msg.From.ID))

		return b.handleMentionMessage(msg)
	}

	// For regular messages, just save and return
	message := &model.Message{
		GroupID:   msg.Chat.ID,
		GroupName: msg.Chat.Title,
		UserID:    msg.From.ID,
		Content:   msg.Text,
		Timestamp: time.Now().UTC(),
	}

	if err := b.store.SaveMessage(b.ctx, message); err != nil {
		return fmt.Errorf("failed to save group message: %w", err)
	}

	return nil
}

func (b *telegramChat) handleMentionMessage(msg *tgbotapi.Message) error {
	// Start typing indicator to provide user feedback
	stopTyping := b.StartTyping(msg.Chat.ID)
	defer close(stopTyping)

	// Collect context for AI response - consolidated logging
	startTime := time.Now()

	// Get user profiles - failure is non-critical
	userProfiles, err := b.store.GetAllUserProfiles(b.ctx)
	if err != nil {
		b.logger.Warn("proceeding with empty user profiles",
			zap.Error(err),
			zap.Int64("chat_id", msg.Chat.ID))

		userProfiles = make(map[int64]*model.UserProfile)
	}

	// Calculate token budget available for message history
	systemPrompt := b.ai.CreateSystemPrompt(userProfiles)
	systemPromptTokens := util.EstimateTokens(systemPrompt)

	// Estimate current message token count
	currentMessageTokens := util.EstimateTokens(msg.Text)

	// Calculate available tokens for history
	availableTokens := b.config.AIMaxContextTokens - systemPromptTokens - currentMessageTokens

	// Retrieve messages in batches until token limit or no more messages
	batchSize := 200
	var beforeTimestamp time.Time // Zero time initially (get latest messages)
	var beforeID uint = 0         // Zero ID initially
	var allMessages []*model.Message
	totalTokens := 0

	for {
		// Get a batch of messages before the current timestamp and ID
		batchMessages, err := b.store.GetRecentMessages(b.ctx, msg.Chat.ID, batchSize, beforeTimestamp, beforeID)
		if err != nil {
			b.sendErrorMessage(msg.Chat.ID)
			return fmt.Errorf("failed to fetch batch of messages: %w", err)
		}

		// If no messages returned, we've retrieved all available messages
		if len(batchMessages) == 0 {
			break
		}

		// Calculate token usage for this batch
		batchTokens := 0
		for _, message := range batchMessages {
			// Add 15 tokens overhead per message for metadata
			msgTokens := util.EstimateTokens(message.Content) + 15
			batchTokens += msgTokens
		}

		// Check if adding this batch would exceed our token budget
		if totalTokens+batchTokens > availableTokens {
			// We would exceed token limit, so don't add all messages from this batch
			// Instead, add messages one by one until we reach the limit
			for _, message := range batchMessages {
				msgTokens := util.EstimateTokens(message.Content) + 15
				if totalTokens+msgTokens <= availableTokens {
					allMessages = append(allMessages, message)
					totalTokens += msgTokens
				} else {
					// Stop adding messages once we exceed the token limit
					break
				}
			}
			break
		}

		// Add all messages from this batch
		allMessages = append(allMessages, batchMessages...)
		totalTokens += batchTokens

		// If we got fewer messages than the batch size, we've reached the end
		if len(batchMessages) < batchSize {
			break
		}

		// Update timestamp and ID for next batch - use the values from the oldest message in this batch
		// The messages are sorted in ascending order by timestamp, so we want the first element
		if len(batchMessages) > 0 {
			oldestMessage := batchMessages[0]
			beforeTimestamp = oldestMessage.Timestamp
			beforeID = oldestMessage.ID
		}
	}

	b.logger.Debug("context collection completed",
		zap.Int64("chat_id", msg.Chat.ID),
		zap.Int("total_messages_retrieved", len(allMessages)),
		zap.Int("profile_count", len(userProfiles)),
		zap.Int("estimated_tokens", totalTokens),
		zap.Int64("duration_ms", time.Since(startTime).Milliseconds()))

	// Generate AI response
	request := &ai.Request{
		UserID:         msg.From.ID,
		Message:        msg.Text,
		RecentMessages: allMessages,
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
	messages := []*model.Message{
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
		if err := b.store.SaveMessage(b.ctx, message); err != nil {
			b.logger.Warn("failed to save message",
				zap.Error(err),
				zap.Int64("chat_id", msg.Chat.ID),
				zap.Bool("is_bot", i == 1))
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

func (b *telegramChat) scheduleMaintenanceTasks() error {
	b.logger.Debug("scheduling daily profile update task", zap.String("cron", "0 0 * * *"))

	err := b.scheduler.AddJob(
		"daily-profile-update",
		"0 0 * * *",
		func() {
			b.logger.Debug("starting scheduled daily profile update")

			startTime := time.Now()

			if err := b.processAndUpdateUserProfiles(); err != nil {
				b.logger.Error("daily profile update failed", zap.Error(err))
			} else {
				duration := time.Since(startTime)
				b.logger.Info("daily profile update completed",
					zap.Int64("duration_ms", duration.Milliseconds()))
			}
		},
	)
	if err != nil {
		b.logger.Error("failed to schedule daily profile update", zap.Error(err))
		return err
	}

	b.logger.Info("maintenance tasks scheduled successfully")
	return nil
}

func (b *telegramChat) processAndUpdateUserProfiles() error {
	b.logger.Debug("starting user profile update process")

	// Retrieve all messages that haven't been processed for user profile analysis yet
	unprocessedMessages, err := b.store.GetUnprocessedMessages(b.ctx)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed messages: %w", err)
	}

	// If there are no unprocessed messages, there's nothing to do
	if len(unprocessedMessages) == 0 {
		b.logger.Debug("no unprocessed messages found, skipping profile update")
		return nil
	}

	b.logger.Debug("retrieved unprocessed messages", zap.Int("count", len(unprocessedMessages)))

	// Get existing user profiles to provide context for the AI analysis
	existingProfiles, err := b.store.GetAllUserProfiles(b.ctx)
	if err != nil {
		// If we can't get existing profiles, start with an empty map
		b.logger.Warn("failed to get existing profiles", zap.Error(err))
		existingProfiles = make(map[int64]*model.UserProfile)
	}

	// Use AI to analyze messages and generate updated user profiles
	b.logger.Debug("generating user profiles with AI",
		zap.Int("message_count", len(unprocessedMessages)),
		zap.Int("existing_profiles", len(existingProfiles)))

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
		if err := b.store.SaveUserProfile(b.ctx, profile); err != nil {
			b.logger.Error("failed to save user profile", zap.Error(err), zap.Int64("user_id", userID))
			return err
		}
	}

	// Mark all processed messages as such to avoid reprocessing them
	messageIDs := make([]uint, 0, len(unprocessedMessages))
	for _, msg := range unprocessedMessages {
		messageIDs = append(messageIDs, msg.ID)
	}

	if err := b.store.MarkMessagesAsProcessed(b.ctx, messageIDs); err != nil {
		return fmt.Errorf("failed to mark messages as processed: %w", err)
	}

	b.logger.Info("user profile update completed", zap.Int("profiles_updated", len(updatedProfiles)))

	return nil
}

// Utility functions

func (b *telegramChat) StartTyping(chatID int64) chan struct{} {
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
							b.logger.Debug("multiple typing action failures",
								zap.Int64("chat_id", chatID),
								zap.Int("count", failureCount))
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

func (b *telegramChat) SendMessage(msg tgbotapi.MessageConfig) error {
	if b.api == nil {
		return errors.New("nil telegram API client")
	}

	// Only log large messages
	if len(msg.Text) > 500 {
		b.logger.Debug("sending large message", zap.Int64("chat_id", msg.ChatID), zap.Int("length", len(msg.Text)))
	}

	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message to chat %d: %w", msg.ChatID, err)
	}

	return nil
}

func (b *telegramChat) sendErrorMessage(chatID int64) {
	reply := tgbotapi.NewMessage(chatID, b.config.BotMsgGeneralError)
	if err := b.SendMessage(reply); err != nil {
		b.logger.Error("failed to send error message", zap.Error(err))
	}
}

func (b *telegramChat) checkAuthorization(msg *tgbotapi.Message) error {
	if !b.IsAuthorized(msg.From.ID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.config.BotMsgNotAuthorized)
		if err := b.SendMessage(reply); err != nil {
			b.logger.Error("failed to send unauthorized message", zap.Error(err))
		}

		return errors.New("unauthorized access attempt")
	}

	return nil
}

func (b *telegramChat) validateMessage(msg *tgbotapi.Message) error {
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

// isCriticalError determines if an error is critical enough to warrant
// terminating the application
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
