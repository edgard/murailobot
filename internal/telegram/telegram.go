package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/rs/zerolog/log"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
)

// Bot encapsulates the Telegram bot logic and its dependencies.
type Bot struct {
	bot     *gotgbot.Bot
	updater *ext.Updater
	db      *db.DB
	oai     *openai.Client
	cfg     *config.Config
}

// NewBot creates a new instance of Bot.
func NewBot(cfg *config.Config, database *db.DB, oaiClient *openai.Client) (*Bot, error) {
	if cfg.TelegramToken == "" || cfg.TelegramAdminUID == 0 {
		return nil, fmt.Errorf("telegram: invalid configuration")
	}
	b, err := gotgbot.NewBot(cfg.TelegramToken, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: failed to create bot: %w", err)
	}

	tg := &Bot{
		bot: b,
		db:  database,
		oai: oaiClient,
		cfg: cfg,
	}
	dispatcher := tg.setupDispatcher()
	tg.updater = ext.NewUpdater(dispatcher, nil)

	commands := []gotgbot.BotCommand{
		{Command: "start", Description: "Start conversation with the bot"},
		{Command: "piu", Description: "Forward an old message"},
		{Command: "mrl", Description: "Generate response using OpenAI"},
		{Command: "mrl_reset", Description: "Reset chat history (admin only)"},
	}
	if _, err = b.SetMyCommands(commands, nil); err != nil {
		return nil, fmt.Errorf("telegram: failed to set commands: %w", err)
	}
	return tg, nil
}

// Stop stops the updater, allowing the bot to terminate gracefully.
func (t *Bot) Stop() {
	t.updater.Stop()
}

// Start begins polling for updates.
func (t *Bot) Start(ctx context.Context) error {
	if err := t.updater.StartPolling(t.bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: 10 * time.Second,
			},
		},
	}); err != nil {
		return fmt.Errorf("telegram: polling error: %w", err)
	}
	log.Info().
		Str("component", "telegram").
		Str("username", t.bot.User.Username).
		Msg("Started Telegram Bot")
	t.updater.Idle() // Blocks until Stop() is called.
	return nil
}

// validateContext ensures that the update context contains the required fields.
func validateContext(ctx *ext.Context) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("telegram: effective message is nil")
	}
	if ctx.EffectiveChat == nil {
		return fmt.Errorf("telegram: effective chat is nil")
	}
	return nil
}

// setupDispatcher registers handlers for various commands and message events.
func (t *Bot) setupDispatcher() *ext.Dispatcher {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Error().
				Err(err).
				Str("component", "telegram").
				Interface("update", ctx.Update).
				Msg("Handler error")
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	dispatcher.AddHandler(handlers.NewCommand("start", t.handleStart))
	dispatcher.AddHandler(handlers.NewCommand("piu", t.handlePiu))
	dispatcher.AddHandler(handlers.NewCommand("mrl", t.handleMrl))
	dispatcher.AddHandler(handlers.NewCommand("mrl_reset", t.handleMrlReset))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, t.handleIncomingMessage))
	return dispatcher
}

// /start command handler.
func (t *Bot) handleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Str("component", "telegram").
		Str("command", "start").
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Processing /start")
	return t.sendMessage(ctx, "Olá! Me encaminhe uma mensagem para guardar.")
}

// /piu command handler.
func (t *Bot) handlePiu(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Str("component", "telegram").
		Str("command", "piu").
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Processing /piu")

	// Use a background context since ext.Context no longer provides one.
	cctx := context.Background()
	user, err := t.db.GetOrCreateUser(cctx, ctx.EffectiveMessage.From.Id, t.cfg.TelegramUserTimeout)
	if err != nil {
		return err
	}
	if time.Since(user.LastUsed).Minutes() <= t.cfg.TelegramUserTimeout {
		log.Info().
			Str("component", "telegram").
			Str("command", "piu").
			Int64("user_id", user.UserID).
			Msg("User is within timeout period")
		return nil
	}
	if err := t.db.UpdateUserLastUsed(cctx, user); err != nil {
		return err
	}
	msgRef, err := t.db.GetRandomMessageRef(cctx)
	if err != nil {
		return err
	}
	return t.forwardMessage(ctx, msgRef.ChatID, msgRef.MessageID)
}

// /mrl command handler.
func (t *Bot) handleMrl(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Str("component", "telegram").
		Str("command", "mrl").
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Processing /mrl")
	if _, err := t.bot.SendChatAction(ctx.EffectiveChat.Id, "typing", nil); err != nil {
		return err
	}
	messageText := strings.TrimSpace(strings.TrimPrefix(ctx.EffectiveMessage.Text, "/mrl"))
	// Use a background context.
	cctx := context.Background()

	history, err := t.db.GetRecentChatHistory(cctx, 30)
	if err != nil {
		return err
	}
	messages := []map[string]string{
		{"role": "system", "content": t.cfg.OpenAIInstruction},
	}
	// Order history chronologically.
	sort.Slice(history, func(i, j int) bool {
		return history[i].LastUsed.Before(history[j].LastUsed)
	})
	for _, entry := range history {
		userName := entry.UserName
		if userName == "" {
			userName = "Unknown User"
		}
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("[UID: %d] %s [%s]: %s", entry.UserID, userName, entry.LastUsed.Format(time.RFC3339), entry.UserMsg),
		})
		messages = append(messages, map[string]string{
			"role":    "assistant",
			"content": entry.BotMsg,
		})
	}
	currentUser := ctx.EffectiveMessage.From
	currentUserName := currentUser.Username
	if currentUserName == "" {
		currentUserName = "Unknown User"
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": fmt.Sprintf("[UID: %d] %s [%s]: %s", currentUser.Id, currentUserName, time.Now().Format(time.RFC3339), messageText),
	})
	reply, err := t.oai.Call(cctx, messages)
	if err != nil {
		return err
	}
	if err := t.sendMessage(ctx, reply); err != nil {
		return err
	}
	historyRecord := db.ChatHistory{
		UserID:   currentUser.Id,
		UserName: currentUser.Username,
		UserMsg:  messageText,
		BotMsg:   reply,
		LastUsed: time.Now(),
	}
	return t.db.AddChatHistory(cctx, historyRecord)
}

// /mrl_reset command handler.
func (t *Bot) handleMrlReset(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Str("component", "telegram").
		Str("command", "mrl_reset").
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Processing /mrl_reset")
	if ctx.EffectiveMessage.From.Id != t.cfg.TelegramAdminUID {
		_, err := ctx.EffectiveMessage.Reply(b, "You are not authorized to use this command.", nil)
		return err
	}
	// Use a background context.
	cctx := context.Background()
	if err := t.db.ClearChatHistory(cctx); err != nil {
		return err
	}
	_, err := ctx.EffectiveMessage.Reply(b, "History has been reset.", nil)
	return err
}

// Handler for forwarded messages.
func (t *Bot) handleIncomingMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	// Only process forwarded messages.
	if ctx.EffectiveMessage.ForwardOrigin == nil {
		log.Info().
			Str("component", "telegram").
			Str("handler", "incoming_message").
			Int64("user_id", ctx.EffectiveMessage.From.Id).
			Msg("Non-forward message ignored")
		return nil
	}
	log.Info().
		Str("component", "telegram").
		Str("handler", "incoming_message").
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Msg("Forwarded message received")
	msgRef := db.MessageRef{
		MessageID: ctx.EffectiveMessage.MessageId,
		ChatID:    ctx.EffectiveMessage.Chat.Id,
		LastUsed:  time.Now(),
	}
	// Use a background context.
	return t.db.AddMessageRef(context.Background(), msgRef)
}

// sendMessage sends a text message as a reply.
func (t *Bot) sendMessage(ctx *ext.Context, text string) error {
	_, err := ctx.EffectiveMessage.Reply(t.bot, text, nil)
	return err
}

// forwardMessage forwards a message from another chat.
func (t *Bot) forwardMessage(ctx *ext.Context, fromChatID, messageID int64) error {
	_, err := t.bot.ForwardMessage(ctx.EffectiveChat.Id, fromChatID, messageID, nil)
	return err
}
