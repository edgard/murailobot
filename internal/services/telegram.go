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
	"golang.org/x/sync/errgroup"
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
	botID          int64
	botUserName    string
	botFirstName   string
}

// Telegram implements the Bot interface for Telegram
type Telegram struct {
	api      *tgbotapi.BotAPI
	config   TelegramConfig
	running  chan struct{}
	handlers map[string]func(context.Context, *tgbotapi.Message) error
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

	config.botID = botUser.ID
	config.botUserName = botUser.UserName
	config.botFirstName = botUser.FirstName

	t := &Telegram{
		api:     bot,
		config:  config,
		running: make(chan struct{}),
	}

	// Initialize command handlers
	t.handlers = map[string]func(context.Context, *tgbotapi.Message) error{
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

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		for {
			select {
			case <-gctx.Done():
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

				g.Go(func() error {
					return t.handleMessage(gctx, update.Message)
				})
			}
		}
	})

	slog.Info("bot started and processing updates")
	return g.Wait()
}

// Stop halts bot operation
func (t *Telegram) Stop() error {
	t.api.StopReceivingUpdates()
	if t.running != nil {
		close(t.running)
	}
	return nil
}

// SendMessage sends a message to a chat
func (t *Telegram) SendMessage(ctx context.Context, chatID int64, text string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		msg := tgbotapi.NewMessage(chatID, text)
		_, err := t.api.Send(msg)
		if err != nil {
			return fmt.Errorf("%w: failed to send message", common.ErrNoResponse)
		}
		return nil
	}
}

// GetID returns the bot's unique identifier
func (t *Telegram) GetID() int64 {
	return t.config.botID
}

// GetUserName returns the bot's username
func (t *Telegram) GetUserName() string {
	return t.config.botUserName
}

// GetFirstName returns the bot's first name
func (t *Telegram) GetFirstName() string {
	return t.config.botFirstName
}

// handleMessage processes an incoming message
func (t *Telegram) handleMessage(ctx context.Context, message *tgbotapi.Message) error {
	if err := t.validateMessage(message); err != nil {
		slog.Error("invalid message", "error", err)
		return nil // Non-critical error
	}

	if message.IsCommand() {
		if err := t.handleCommand(ctx, message); err != nil {
			slog.Error("command error", "error", err)
			return err
		}
		return nil
	}

	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		if err := t.handleGroupMessage(ctx, message); err != nil {
			slog.Error("group message error", "error", err)
			return err
		}
	}

	return nil
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

// handleCommand processes bot commands
func (t *Telegram) handleCommand(ctx context.Context, message *tgbotapi.Message) error {
	cmd := message.Command()

	handler, exists := t.handlers[cmd]
	if !exists {
		return nil
	}

	if err := handler(ctx, message); err != nil {
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
func (t *Telegram) handleStartCommand(ctx context.Context, message *tgbotapi.Message) error {
	return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["welcome"])
}

// handleResetCommand resets chat history
func (t *Telegram) handleResetCommand(ctx context.Context, message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["unauthorized"])
	}

	if err := t.config.DB.DeleteMessages(ctx, message.Chat.ID); err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageDelete, err)
	}

	return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["reset"])
}

// handleAnalyzeCommand analyzes messages and updates profiles
func (t *Telegram) handleAnalyzeCommand(ctx context.Context, message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["unauthorized"])
	}

	if err := t.SendMessage(ctx, message.Chat.ID, t.config.Templates["analyzing"]); err != nil {
		return err
	}

	// Get unprocessed messages
	messages, err := t.config.DB.GetUnprocessedMessages(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}

	if len(messages) == 0 {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["no_profiles"])
	}

	// Get existing profiles for context and merging
	existingProfiles, err := t.config.DB.GetAllProfiles(ctx)
	if err != nil {
		slog.Warn("proceeding with empty profiles", "error", err)
		existingProfiles = make(map[int64]*models.UserProfile)
	}

	// Process messages by user
	userMessages := make(map[int64][]*models.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	// Update profiles using errgroup for concurrency
	g, gctx := errgroup.WithContext(ctx)
	var messageIDs []uint

	for userID, msgs := range userMessages {
		msgs := msgs // Capture for goroutine
		userID := userID
		g.Go(func() error {
			profile, err := t.config.AI.GenerateProfile(gctx, userID, msgs)
			if err != nil {
				slog.Error("failed to generate profile",
					"error", err,
					"user_id", userID)
				return nil // Non-critical error
			}

			if existingProfile, ok := existingProfiles[userID]; ok {
				t.mergeProfiles(profile, existingProfile)
			}

			for _, msg := range msgs {
				messageIDs = append(messageIDs, msg.ID)
			}

			if err := t.config.DB.SaveProfile(gctx, profile); err != nil {
				slog.Error("failed to save profile",
					"error", err,
					"user_id", userID)
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		slog.Error("error processing profiles", "error", err)
	}

	if len(messageIDs) > 0 {
		if err := t.config.DB.MarkMessagesAsProcessed(ctx, messageIDs); err != nil {
			slog.Error("failed to mark messages as processed", "error", err)
		}
	}

	// Show updated profiles
	return t.handleProfilesCommand(ctx, message)
}

// mergeProfiles merges an existing profile into a new one
func (t *Telegram) mergeProfiles(new, existing *models.UserProfile) {
	if new.DisplayNames == "" {
		new.DisplayNames = existing.DisplayNames
	}
	if new.OriginLocation == "" {
		new.OriginLocation = existing.OriginLocation
	}
	if new.CurrentLocation == "" {
		new.CurrentLocation = existing.CurrentLocation
	}
	if new.AgeRange == "" {
		new.AgeRange = existing.AgeRange
	}
	if new.Traits == "" {
		new.Traits = existing.Traits
	}
	new.CreatedAt = existing.CreatedAt
	new.ID = existing.ID
}

// handleProfilesCommand shows user profiles
func (t *Telegram) handleProfilesCommand(ctx context.Context, message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["unauthorized"])
	}

	profiles, err := t.config.DB.GetAllProfiles(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileFetch, err)
	}

	if len(profiles) == 0 {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["no_profiles"])
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

	return t.SendMessage(ctx, message.Chat.ID, b.String())
}

// handleEditCommand edits user profile data
func (t *Telegram) handleEditCommand(ctx context.Context, message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["unauthorized"])
	}

	args := strings.Fields(message.CommandArguments())
	if len(args) < 3 {
		return t.SendMessage(ctx, message.Chat.ID, "Usage: /edit <user_id> <field> <value>")
	}

	var userID int64
	if _, err := fmt.Sscanf(args[0], "%d", &userID); err != nil {
		return t.SendMessage(ctx, message.Chat.ID, "Invalid user ID")
	}

	field := args[1]
	value := strings.Join(args[2:], " ")

	profile, err := t.config.DB.GetProfile(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileFetch, err)
	}
	if profile == nil {
		return t.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("No profile found for user ID %d", userID))
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
		return t.SendMessage(ctx, message.Chat.ID, "Invalid field name")
	}

	if err := t.config.DB.SaveProfile(ctx, profile); err != nil {
		return fmt.Errorf("%w: %v", common.ErrProfileUpdate, err)
	}

	return t.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("Updated profile for user ID %d", userID))
}

// handleGroupMessage processes a message from a group chat
func (t *Telegram) handleGroupMessage(ctx context.Context, message *tgbotapi.Message) error {
	// Check for bot mention
	if !strings.Contains(message.Text, "@"+t.config.botUserName) {
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

	// Get conversation history with fixed size limit
	messages, err := t.config.DB.GetMessages(ctx, message.Chat.ID, t.config.MaxContextSize, time.Now())
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}

	// Generate AI response
	response, err := t.config.AI.GenerateResponse(ctx, messages)
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
			UserID:    t.config.botID,
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
	if err := t.SendMessage(ctx, message.Chat.ID, response); err != nil {
		return fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	return nil
}
