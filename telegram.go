package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/rs/zerolog/log"
)

// Telegram encapsulates the bot's logic and dependencies.
type Telegram struct {
	bot     *gotgbot.Bot // Telegram bot instance.
	updater *ext.Updater // Updater for handling updates.
	db      *DB          // Database interface.
	oai     *OpenAI      // OpenAI interface.
	config  *Config      // Application configuration.
}

// NewTelegram creates and initializes a new Telegram bot instance.
func NewTelegram(config *Config, db *DB, oai *OpenAI) (*Telegram, error) {
	if config.TelegramToken == "" || config.TelegramAdminUID == 0 {
		return nil, WrapError("invalid Telegram configuration")
	}

	bot, err := gotgbot.NewBot(config.TelegramToken, nil)
	if err != nil {
		return nil, WrapError("failed to create new bot", err)
	}

	tg := &Telegram{
		bot:    bot,
		db:     db,
		oai:    oai,
		config: config,
	}
	tg.updater = ext.NewUpdater(tg.setupDispatcher(), nil)

	commands := []gotgbot.BotCommand{
		{Command: "start", Description: "Iniciar conversa o bot"},
		{Command: "piu", Description: "Enviar forward de uma mensagem antiga"},
		{Command: "mrl", Description: "Gerar uma resposta usando OpenAI"},
		{Command: "mrl_reset", Description: "Limpar histórico de mensagens (apenas admin)"},
	}
	if _, err = bot.SetMyCommands(commands, nil); err != nil {
		return nil, WrapError("failed to set bot commands", err)
	}

	return tg, nil
}

// Start begins polling for updates.
func (tg *Telegram) Start() error {
	if err := tg.updater.StartPolling(tg.bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: 10 * time.Second,
			},
		},
	}); err != nil {
		return WrapError("failed to start polling", err)
	}

	log.Info().Str("username", tg.bot.User.Username).Msg("Started Telegram Bot")
	tg.updater.Idle()
	return nil
}

// validateContext ensures EffectiveMessage and EffectiveChat are present.
func validateContext(ctx *ext.Context) error {
	if ctx.EffectiveMessage == nil {
		return WrapError("effective message is nil")
	}
	if ctx.EffectiveChat == nil {
		return WrapError("effective chat is nil")
	}
	return nil
}

// setupDispatcher configures the update dispatcher.
func (tg *Telegram) setupDispatcher() *ext.Dispatcher {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Error().Err(err).Msg("Error occurred while handling update")
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	dispatcher.AddHandler(handlers.NewCommand("start", tg.handleStartRequest))
	dispatcher.AddHandler(handlers.NewCommand("piu", tg.handlePiuRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl", tg.handleMrlRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl_reset", tg.handleMrlResetRequest))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, tg.handleIncomingMessage))
	return dispatcher
}

// handleIncomingMessage processes forwarded messages.
func (tg *Telegram) handleIncomingMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	if ctx.EffectiveMessage.ForwardOrigin == nil {
		log.Info().
			Int64("user_id", ctx.EffectiveMessage.From.Id).
			Str("username", ctx.EffectiveMessage.From.Username).
			Int64("update_id", ctx.Update.UpdateId).
			Msg("Received non-forward message, ignoring")
		return nil
	}

	log.Info().
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Str("username", ctx.EffectiveMessage.From.Username).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Received forward message")

	msgRef := MessageRef{
		MessageID: ctx.EffectiveMessage.MessageId,
		ChatID:    ctx.EffectiveMessage.Chat.Id,
		LastUsed:  time.Now(),
	}
	if err := tg.db.AddMessageRef(&msgRef); err != nil {
		return WrapError("failed to add message reference to database", err)
	}

	return tg.sendTelegramMessage(ctx, "Mensagem adicionada ao banco de dados!")
}

// handleStartRequest processes the /start command.
func (tg *Telegram) handleStartRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Str("username", ctx.EffectiveMessage.From.Username).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Received START request")
	return tg.sendTelegramMessage(ctx, "Olá! Me encaminhe uma mensagem para guardar.")
}

// handlePiuRequest processes the /piu command.
func (tg *Telegram) handlePiuRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Str("username", ctx.EffectiveMessage.From.Username).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Received PIU request")

	user, err := tg.db.GetOrCreateUser(ctx.EffectiveMessage.From.Id, tg.config.TelegramUserTimeout)
	if err != nil {
		return WrapError("failed to get or create user", err)
	}
	if time.Since(user.LastUsed).Minutes() <= tg.config.TelegramUserTimeout {
		log.Info().
			Int64("user_id", user.UserID).
			Str("username", ctx.EffectiveMessage.From.Username).
			Time("last_used", user.LastUsed).
			Msg("User on timeout")
		return nil
	}
	if err := tg.db.UpdateUserLastUsed(user); err != nil {
		return WrapError("failed to update user's last used time", err)
	}
	msgRef, err := tg.db.GetRandomMessageRef()
	if err != nil {
		return WrapError("failed to get random message reference", err)
	}
	return tg.forwardTelegramMessage(ctx, msgRef.ChatID, msgRef.MessageID)
}

// handleMrlRequest processes the /mrl command for generating a response using OpenAI.
func (tg *Telegram) handleMrlRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Str("username", ctx.EffectiveMessage.From.Username).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Received MRL request")
	if _, err := tg.bot.SendChatAction(ctx.EffectiveChat.Id, "typing", nil); err != nil {
		return WrapError("failed to send chat action", err)
	}
	messageText := strings.TrimSpace(strings.TrimPrefix(ctx.EffectiveMessage.Text, "/mrl"))
	gptHistory, err := tg.db.GetRecentChatHistory(30)
	if err != nil {
		return WrapError("failed to get recent chat history", err)
	}
	messages := []map[string]string{
		{"role": "system", "content": tg.config.OpenAIInstruction},
	}
	sort.Slice(gptHistory, func(i, j int) bool {
		return gptHistory[i].LastUsed.Before(gptHistory[j].LastUsed)
	})
	for _, history := range gptHistory {
		userName := history.UserName
		if userName == "" {
			userName = "Unknown User"
		}
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("[UID: %d] %s [%s]: %s", history.UserID, userName, history.LastUsed.Format(time.RFC3339), history.UserMsg),
		})
		messages = append(messages, map[string]string{
			"role":    "assistant",
			"content": history.BotMsg,
		})
	}
	currentUserName := ctx.EffectiveMessage.From.Username
	if currentUserName == "" {
		currentUserName = "Unknown User"
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": fmt.Sprintf("[UID: %d] %s [%s]: %s", ctx.EffectiveMessage.From.Id, currentUserName, time.Now().Format(time.RFC3339), messageText),
	})
	content, err := tg.oai.Call(messages)
	if err != nil {
		return WrapError("failed to call OpenAI", err)
	}
	if err := tg.sendTelegramMessage(ctx, content); err != nil {
		return WrapError("failed to send OpenAI response", err)
	}
	historyRecord := ChatHistory{
		UserID:   ctx.EffectiveMessage.From.Id,
		UserName: ctx.EffectiveMessage.From.Username,
		UserMsg:  messageText,
		BotMsg:   content,
		LastUsed: time.Now(),
	}
	return tg.db.AddChatHistory(&historyRecord)
}

// handleMrlResetRequest processes the /mrl_reset command to clear chat history (admin only).
func (tg *Telegram) handleMrlResetRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	log.Info().
		Int64("user_id", ctx.EffectiveMessage.From.Id).
		Str("username", ctx.EffectiveMessage.From.Username).
		Int64("update_id", ctx.Update.UpdateId).
		Msg("Received MRL_RESET request")
	if ctx.EffectiveMessage.From.Id != tg.config.TelegramAdminUID {
		if _, err := ctx.EffectiveMessage.Reply(b, "You are not authorized to use this command.", nil); err != nil {
			return WrapError("failed to send unauthorized message", err)
		}
		return nil
	}
	if err := tg.db.ClearChatHistory(); err != nil {
		return WrapError("failed to clear chat history", err)
	}
	_, err := ctx.EffectiveMessage.Reply(b, "History has been reset.", nil)
	return WrapError("failed to send reset confirmation message", err)
}

// sendTelegramMessage replies with a text message in the current chat.
func (tg *Telegram) sendTelegramMessage(ctx *ext.Context, text string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	_, err := ctx.EffectiveMessage.Reply(tg.bot, text, nil)
	return WrapError("failed to send telegram message", err)
}

// forwardTelegramMessage forwards a message from a different chat.
func (tg *Telegram) forwardTelegramMessage(ctx *ext.Context, forwardChatID, forwardMessageID int64) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	_, err := tg.bot.ForwardMessage(ctx.EffectiveChat.Id, forwardChatID, forwardMessageID, nil)
	return WrapError("failed to forward telegram message", err)
}
