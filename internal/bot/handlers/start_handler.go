package handlers

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewStartHandler returns a handler for the /start command.
func NewStartHandler(deps HandlerDeps) bot.HandlerFunc {
	return startHandler{deps}.Handle
}

// startHandler processes the /start command using injected dependencies.
type startHandler struct {
	deps HandlerDeps
}

func (h startHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "start")

	if update.Message == nil || update.Message.From == nil {
		log.WarnContext(ctx, "Start handler received update with nil message or sender", "update_id", update.ID)
		return
	}

	log.InfoContext(ctx, "Handling /start command", "chat_id", update.Message.Chat.ID, "user_id", update.Message.From.ID)

	welcome := h.deps.Config.Messages.Welcome
	if h.deps.Config.Telegram.BotInfo.Username != "" {
		welcome = strings.ReplaceAll(welcome, "@botname", "@"+h.deps.Config.Telegram.BotInfo.Username)
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: welcome})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send welcome message", "error", err, "chat_id", update.Message.Chat.ID)
	} else {
		log.DebugContext(ctx, "Successfully sent help message", "chat_id", update.Message.Chat.ID)
	}
}
