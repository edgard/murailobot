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

// Telegram implements the Bot interface for Telegram
type Telegram struct {
	api      *tgbotapi.BotAPI
	running  chan struct{}
	handlers map[string]func(context.Context, *tgbotapi.Message) error
	config   struct {
		Token          string
		AdminID        int64
		MaxContextSize int
		AI             interfaces.AI
		DB             interfaces.DB
		Scheduler      interfaces.Scheduler
		Commands       map[string]string
		Templates      map[string]string
		botID          int64
		botUserName    string
		botFirstName   string
	}
}

// NewTelegram creates a new Telegram bot instance
func NewTelegram() (interfaces.Bot, error) {
	return &Telegram{
		running: make(chan struct{}),
		handlers: map[string]func(context.Context, *tgbotapi.Message) error{
			"start":    nil,
			"reset":    nil,
			"analyze":  nil,
			"profiles": nil,
			"edit":     nil,
		},
	}, nil
}

// Configure sets up the bot with basic configuration
func (t *Telegram) Configure(token string, adminID int64, maxContextSize int) error {
	if token == "" {
		return common.ErrMissingToken
	}

	if maxContextSize == 0 {
		maxContextSize = 50
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrInitialization, err)
	}

	botUser, err := bot.GetMe()
	if err != nil {
		return fmt.Errorf("%w: failed to get bot info", common.ErrInitialization)
	}

	t.api = bot
	t.config.Token = token
	t.config.AdminID = adminID
	t.config.MaxContextSize = maxContextSize
	t.config.botID = botUser.ID
	t.config.botUserName = botUser.UserName
	t.config.botFirstName = botUser.FirstName

	// Initialize command handlers
	t.handlers = map[string]func(context.Context, *tgbotapi.Message) error{
		"start":    t.handleStartCommand,
		"reset":    t.handleResetCommand,
		"analyze":  t.handleAnalyzeCommand,
		"profiles": t.handleProfilesCommand,
		"edit":     t.handleEditCommand,
	}

	return nil
}

// GetBotInfo returns bot identification information
func (t *Telegram) GetBotInfo() models.BotInfo {
	return models.BotInfo{
		ID:        t.config.botID,
		UserName:  t.config.botUserName,
		FirstName: t.config.botFirstName,
	}
}

// SetServices configures bot with required service dependencies
func (t *Telegram) SetServices(ai interfaces.AI, db interfaces.DB, scheduler interfaces.Scheduler) error {
	t.config.AI = ai
	t.config.DB = db
	t.config.Scheduler = scheduler
	return nil
}

// SetCommands sets available bot commands
func (t *Telegram) SetCommands(commands map[string]string) error {
	t.config.Commands = commands
	return nil
}

// SetTemplates sets message templates
func (t *Telegram) SetTemplates(templates map[string]string) error {
	t.config.Templates = templates
	return nil
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

				// Skip non-text messages or empty messages
				if update.Message == nil || update.Message.Text == "" {
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

// handleMessage processes an incoming message
func (t *Telegram) handleMessage(ctx context.Context, message *tgbotapi.Message) error {
	if message.IsCommand() {
		return t.handleCommand(ctx, message)
	}

	if message.Chat.IsGroup() || message.Chat.IsSuperGroup() {
		return t.handleGroupMessage(ctx, message)
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

	err := handler(ctx, message)
	if err == common.ErrUnauthorized {
		slog.Warn("unauthorized command attempt",
			"command", cmd,
			"user_id", message.From.ID,
			"chat_id", message.Chat.ID)
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["unauthorized"])
	}
	if err != nil {
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
		return common.ErrUnauthorized
	}

	if err := t.config.DB.DeleteMessages(ctx, message.Chat.ID); err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageDelete, err)
	}

	return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["reset"])
}

// handleAnalyzeCommand analyzes messages and updates profiles
func (t *Telegram) handleAnalyzeCommand(ctx context.Context, message *tgbotapi.Message) error {
	if message.From.ID != t.config.AdminID {
		return common.ErrUnauthorized
	}

	if err := t.SendMessage(ctx, message.Chat.ID, t.config.Templates["analyzing"]); err != nil {
		return err
	}

	messages, err := t.config.DB.GetUnprocessedMessages(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}

	if len(messages) == 0 {
		return t.SendMessage(ctx, message.Chat.ID, t.config.Templates["no_profiles"])
	}

	existingProfiles, err := t.config.DB.GetAllProfiles(ctx)
	if err != nil {
		slog.Warn("proceeding with empty profiles", "error", err)
		existingProfiles = make(map[int64]*models.UserProfile)
	}

	userMessages := make(map[int64][]*models.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	g, gctx := errgroup.WithContext(ctx)
	var messageIDs []uint

	for userID, msgs := range userMessages {
		msgs := msgs
		userID := userID
		g.Go(func() error {
			profile, err := t.config.AI.GenerateProfile(gctx, userID, msgs, t.GetBotInfo())
			if err != nil {
				slog.Error("failed to generate profile",
					"error", err,
					"user_id", userID)
				return nil
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
		return common.ErrUnauthorized
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
		return common.ErrUnauthorized
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
	if !strings.Contains(message.Text, "@"+t.config.botUserName) {
		// For non-mention messages, save and return
		msg := &models.Message{
			UserID:    message.From.ID,
			GroupID:   message.Chat.ID,
			Content:   message.Text,
			CreatedAt: time.Now().UTC(),
			IsFromBot: false,
		}

		if err := t.config.DB.SaveMessage(ctx, msg); err != nil {
			return fmt.Errorf("%w: %v", common.ErrMessageSave, err)
		}

		return nil
	}

	messages, err := t.config.DB.GetMessages(ctx, message.Chat.ID, t.config.MaxContextSize, time.Now())
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrMessageFetch, err)
	}

	response, err := t.config.AI.GenerateResponse(ctx, messages, t.GetBotInfo())
	if err != nil {
		return fmt.Errorf("%w: %v", common.ErrNoResponse, err)
	}

	now := time.Now().UTC()
	msgs := []*models.Message{
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

	for _, msg := range msgs {
		if err := t.config.DB.SaveMessage(ctx, msg); err != nil {
			slog.Error("failed to save message",
				"error", err,
				"is_bot", msg.IsFromBot)
		}
	}

	return t.SendMessage(ctx, message.Chat.ID, response)
}
