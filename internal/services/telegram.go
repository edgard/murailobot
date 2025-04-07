package services

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/common"
	"github.com/edgard/murailobot/internal/interfaces"
	"github.com/edgard/murailobot/internal/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramConfig holds configuration for Telegram bot
type TelegramConfig struct {
	Token          string
	AdminID        int64
	AI             interfaces.AI
	DB             interfaces.DB
	Scheduler      interfaces.Scheduler
	Commands       map[string]string // Command -> Description
	Templates      map[string]string // Template name -> Message template
	MaxContextSize int               // Maximum number of messages for context
}

// Telegram implements the Bot interface for Telegram
type Telegram struct {
	api         *tgbotapi.BotAPI
	config      TelegramConfig
	userID      int64
	username    string
	displayName string
	ctx         context.Context
	running     chan struct{}
	handlers    map[string]func(*tgbotapi.Message) error
}

// messageProcessor encapsulates context for message processing
type messageProcessor struct {
	chatID   int64
	userID   int64
	text     string
	isBot    bool
	profiles map[int64]*models.UserProfile
	messages []*models.Message
}

// NewTelegram creates a new Telegram bot instance
func NewTelegram(config TelegramConfig) (interfaces.Bot, error) {
	if config.Token == "" {
		return nil, common.ErrMissingToken
	}

	if config.MaxContextSize == 0 {
		config.MaxContextSize = 50
	}

	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", common.ErrInitialization, err)
	}

	botUser, err := bot.GetMe()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get bot info", common.ErrInitialization)
	}

	t := &Telegram{
		api:         bot,
		config:      config,
		userID:      botUser.ID,
		username:    botUser.UserName,
		displayName: botUser.FirstName,
		running:     make(chan struct{}),
	}

	// Initialize command handlers
	t.handlers = map[string]func(*tgbotapi.Message) error{
		"start":    t.handleStartCommand,
		"reset":    t.handleResetCommand,
		"analyze":  t.handleAnalyzeCommand,
		"profiles": t.handleProfilesCommand,
		"edit":     t.handleEditCommand,
	}

	return t, nil
}

// Start begins bot operation
func (t *Telegram) Start(ctx context.Context) error {
	t.ctx = ctx

	// Set up bot commands
	commands := make([]tgbotapi.BotCommand, 0, len(t.config.Commands))
	for cmd, desc := range t.config.Commands {
		commands = append(commands, tgbotapi.BotCommand{
			Command:     strings.TrimPrefix(cmd, "/"),
			Description: desc,
		})
	}

	if _, err := t.api.Request(tgbotapi.NewSetMyCommands(commands...)); err != nil {
		return fmt.Errorf("%w: failed to set commands", common.ErrServiceStart)
	}

	// Configure update channel
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := t.api.GetUpdatesChan(u)

	slog.Info("bot started and processing updates")

	for {
		select {
		case <-ctx.Done():
			close(t.running)
			return ctx.Err()

		case update, ok := <-updates:
			if !ok {
				slog.Info("update channel closed")
				return nil
			}

			if update.Message == nil {
				continue
			}

			go t.handleMessage(ctx, update.Message)
		}
	}
}

// Stop halts bot operation
func (t *Telegram) Stop() error {
	t.api.StopReceivingUpdates()
	if t.running != nil {
		close(t.running)
	}
	return nil
}

// SetName updates bot name information
func (t *Telegram) SetName(username, displayName string) {
	t.username = username
	t.displayName = displayName
}

// GetInfo returns the bot's identification information
func (t *Telegram) GetInfo() interfaces.BotInfo {
	return interfaces.BotInfo{
		UserID:      t.userID,
		Username:    t.username,
		DisplayName: t.displayName,
	}
}

// SendMessage sends a message to a chat
func (t *Telegram) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := t.api.Send(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to send message", common.ErrNoResponse)
	}
	return nil
}

// handleMessage processes an incoming message
func (t *Telegram) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	if err := t.validateMessage(message); err != nil {
		slog.Error("invalid message", "error", err)
		return
	}

	if message.IsCommand() {
		if err := t.handleCommand(ctx, message); err != nil {
			if t.isCriticalError(err) {
				slog.Error("critical command error",
					"error", err,
					"command", message.Command(),
					"chat_id", message.Chat.ID)
			} else {
				slog.Error("command error",
					"error", err,
					"command", message.Command())
			}
		}
		return
	}

	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		if err := t.handleGroupMessage(ctx, message); err != nil {
			if t.isCriticalError(err) {
				slog.Error("critical error in group message",
					"error", err,
					"chat_id", message.Chat.ID)
				return
			}
			slog.Error("group message error",
				"error", err,
				"chat_id", message.Chat.ID)
		}
	}
}

// validateMessage performs message validation
func (t *Telegram) validateMessage(msg *tgbotapi.Message) error {
	if msg == nil || msg.Text == "" {
		return fmt.Errorf("%w: empty message", common.ErrInvalidMessage)
	}

	if msg.Chat == nil {
		return fmt.Errorf("%w: nil chat", common.ErrInvalidMessage)
	}

	if msg.From == nil {
		return fmt.Errorf("%w: nil sender", common.ErrInvalidMessage)
	}

	if msg.Chat.ID == 0 || msg.From.ID == 0 {
		return fmt.Errorf("%w: invalid chat or user ID", common.ErrInvalidMessage)
	}

	return nil
}

// isCriticalError determines if an error is critical
func (t *Telegram) isCriticalError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "database connection") ||
		strings.Contains(err.Error(), "token revoked")
}

// checkAuthorization verifies admin access
func (t *Telegram) checkAuthorization(message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		if err := t.SendMessage(message.Chat.ID, t.config.Templates["unauthorized"]); err != nil {
			slog.Error("failed to send unauthorized message", "error", err)
		}
		return common.ErrUnauthorized
	}
	return nil
}

// startTyping sends typing indicator until stopped
func (t *Telegram) startTyping(chatID int64) chan struct{} {
	stopTyping := make(chan struct{})

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = t.api.Request(action)

	ctx, cancel := context.WithCancel(t.ctx)

	go func() {
		defer cancel()

		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopTyping:
				return
			case <-t.running:
				return
			case <-ticker.C:
				select {
				case <-ctx.Done():
					return
				default:
					_, _ = t.api.Request(action)
				}
			}
		}
	}()

	return stopTyping
}

// handleGroupMessage processes a message from a group chat
func (t *Telegram) handleGroupMessage(ctx context.Context, message *tgbotapi.Message) error {
	// Check for bot mention
	botMention := "@" + t.username
	if !strings.Contains(message.Text, botMention) {
		// For non-mention messages, just save and return
		msg := &models.Message{
			UserID:    message.From.ID,
			GroupID:   message.Chat.ID,
			Content:   message.Text,
			CreatedAt: time.Now().UTC(),
			IsFromBot: false,
		}

		if err := t.config.DB.SaveMessage(ctx, msg); err != nil {
			if strings.Contains(err.Error(), "database connection") {
				return fmt.Errorf("%w: %v", common.ErrDatabaseConnection, err)
			}
			return fmt.Errorf("%w: %v", common.ErrMessageSave, err)
		}

		return nil
	}

	// Start typing indicator
	stopTyping := t.startTyping(message.Chat.ID)
	defer close(stopTyping)

	// Initialize message processor
	proc := &messageProcessor{
		chatID: message.Chat.ID,
		userID: message.From.ID,
		text:   message.Text,
	}

	// Get user profiles for context
	profiles, err := t.config.DB.GetAllProfiles(ctx)
	if err != nil {
		slog.Warn("proceeding with empty user profiles",
			"error", err,
			"chat_id", message.Chat.ID)
		profiles = make(map[int64]*models.UserProfile)
	}
	proc.profiles = profiles

	// Get conversation history with fixed size limit
	messages, err := t.config.DB.GetMessages(ctx, message.Chat.ID, t.config.MaxContextSize, time.Now())
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}
	proc.messages = messages

	// Generate AI response
	response, err := t.config.AI.GenerateResponse(ctx, proc.messages)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	// Save both messages
	now := time.Now().UTC()
	messages = []*models.Message{
		{
			UserID:    message.From.ID,
			GroupID:   message.Chat.ID,
			Content:   message.Text,
			CreatedAt: now,
			IsFromBot: false,
		},
		{
			UserID:    t.userID,
			GroupID:   message.Chat.ID,
			Content:   response,
			CreatedAt: now,
			IsFromBot: true,
		},
	}

	for _, msg := range messages {
		if err := t.config.DB.SaveMessage(ctx, msg); err != nil {
			slog.Error("failed to save message",
				"error", err,
				"is_bot", msg.IsFromBot)
		}
	}

	// Send the response
	if err := t.SendMessage(message.Chat.ID, response); err != nil {
		return fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	return nil
}

// handleCommand processes bot commands
func (t *Telegram) handleCommand(ctx context.Context, message *tgbotapi.Message) error {
	cmd := message.Command()

	handler, exists := t.handlers[cmd]
	if !exists {
		return nil
	}

	if err := handler(message); err != nil {
		if err == common.ErrUnauthorized {
			slog.Warn("unauthorized command attempt",
				"command", cmd,
				"user_id", message.From.ID,
				"chat_id", message.Chat.ID)
		}
		return fmt.Errorf("%w: command '%s' failed", err, cmd)
	}

	return nil
}

// handleStartCommand processes the start command
func (t *Telegram) handleStartCommand(message *tgbotapi.Message) error {
	return t.SendMessage(message.Chat.ID, t.config.Templates["welcome"])
}

// handleResetCommand resets chat history
func (t *Telegram) handleResetCommand(message *tgbotapi.Message) error {
	if err := t.checkAuthorization(message); err != nil {
		return err
	}

	if err := t.config.DB.DeleteMessages(t.ctx, message.Chat.ID); err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageDelete, err)
	}

	return t.SendMessage(message.Chat.ID, t.config.Templates["reset"])
}

// handleAnalyzeCommand analyzes messages and updates profiles
func (t *Telegram) handleAnalyzeCommand(message *tgbotapi.Message) error {
	if err := t.checkAuthorization(message); err != nil {
		return err
	}

	if err := t.SendMessage(message.Chat.ID, t.config.Templates["analyzing"]); err != nil {
		return err
	}

	stopTyping := t.startTyping(message.Chat.ID)
	defer close(stopTyping)

	// Get unprocessed messages
	messages, err := t.config.DB.GetUnprocessedMessages(t.ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}

	if len(messages) == 0 {
		return t.SendMessage(message.Chat.ID, t.config.Templates["no_profiles"])
	}

	// Get existing profiles for context and merging
	existingProfiles, err := t.config.DB.GetAllProfiles(t.ctx)
	if err != nil {
		slog.Warn("proceeding with empty profiles", "error", err)
		existingProfiles = make(map[int64]*models.UserProfile)
	}

	// Process messages by user
	userMessages := make(map[int64][]*models.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	// Update profiles
	var messageIDs []uint
	for userID, msgs := range userMessages {
		// Generate new profile
		profile, err := t.config.AI.GenerateProfile(t.ctx, userID, msgs)
		if err != nil {
			slog.Error("failed to generate profile",
				"error", err,
				"user_id", userID)
			continue
		}

		// Merge with existing profile if available
		if existingProfile, ok := existingProfiles[userID]; ok {
			if profile.DisplayNames == "" {
				profile.DisplayNames = existingProfile.DisplayNames
			}
			if profile.OriginLocation == "" {
				profile.OriginLocation = existingProfile.OriginLocation
			}
			if profile.CurrentLocation == "" {
				profile.CurrentLocation = existingProfile.CurrentLocation
			}
			if profile.AgeRange == "" {
				profile.AgeRange = existingProfile.AgeRange
			}
			if profile.Traits == "" {
				profile.Traits = existingProfile.Traits
			}
			profile.CreatedAt = existingProfile.CreatedAt
			profile.ID = existingProfile.ID
		}

		// Collect message IDs for marking as processed
		for _, msg := range msgs {
			messageIDs = append(messageIDs, msg.ID)
		}

		if err := t.config.DB.SaveProfile(t.ctx, profile); err != nil {
			slog.Error("failed to save profile",
				"error", err,
				"user_id", userID)
			continue
		}
	}

	if len(messageIDs) > 0 {
		if err := t.config.DB.MarkMessagesAsProcessed(t.ctx, messageIDs); err != nil {
			slog.Error("failed to mark messages as processed", "error", err)
		}
	}

	// Show updated profiles
	return t.handleProfilesCommand(message)
}

// handleProfilesCommand shows user profiles
func (t *Telegram) handleProfilesCommand(message *tgbotapi.Message) error {
	if err := t.checkAuthorization(message); err != nil {
		return err
	}

	profiles, err := t.config.DB.GetAllProfiles(t.ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileFetch, err)
	}

	if len(profiles) == 0 {
		return t.SendMessage(message.Chat.ID, t.config.Templates["no_profiles"])
	}

	var b strings.Builder
	b.WriteString(t.config.Templates["profiles_header"])

	userIDs := make([]int64, 0, len(profiles))
	for id := range profiles {
		userIDs = append(userIDs, id)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	for _, id := range userIDs {
		p := profiles[id]
		fmt.Fprintf(&b, "UID %d (%s):\n", p.UserID, p.DisplayNames)
		fmt.Fprintf(&b, "Origin: %s\n", p.OriginLocation)
		fmt.Fprintf(&b, "Current: %s\n", p.CurrentLocation)
		fmt.Fprintf(&b, "Age: %s\n", p.AgeRange)
		fmt.Fprintf(&b, "Traits: %s\n\n", p.Traits)
	}

	return t.SendMessage(message.Chat.ID, b.String())
}

// handleEditCommand edits user profile data
func (t *Telegram) handleEditCommand(message *tgbotapi.Message) error {
	if err := t.checkAuthorization(message); err != nil {
		return err
	}

	args := strings.Fields(message.CommandArguments())
	if len(args) < 3 {
		return t.SendMessage(message.Chat.ID, "Usage: /edit <user_id> <field> <value>")
	}

	var userID int64
	if _, err := fmt.Sscanf(args[0], "%d", &userID); err != nil {
		return t.SendMessage(message.Chat.ID, "Invalid user ID")
	}

	field := args[1]
	value := strings.Join(args[2:], " ")

	profile, err := t.config.DB.GetProfile(t.ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileFetch, err)
	}
	if profile == nil {
		return t.SendMessage(message.Chat.ID, fmt.Sprintf("No profile found for user ID %d", userID))
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
		return t.SendMessage(message.Chat.ID, "Invalid field name")
	}

	if err := t.config.DB.SaveProfile(t.ctx, profile); err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileUpdate, err)
	}

	return t.SendMessage(message.Chat.ID, fmt.Sprintf("Updated profile for user ID %d", userID))
}
