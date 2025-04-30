package handlers

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewHelpHandler returns a handler for the /help command.
func NewHelpHandler(deps HandlerDeps) bot.HandlerFunc {
	return helpHandler{deps}.Handle
}

// helpHandler processes the /help command using injected dependencies.
type helpHandler struct {
	deps HandlerDeps
}

func (h helpHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "help")

	if update.Message == nil || update.Message.From == nil {
		log.WarnContext(ctx, "Help handler received update with nil message or sender", "update_id", update.ID)
		return
	}

	log.InfoContext(ctx, "Handling /help command", "chat_id", update.Message.Chat.ID, "user_id", update.Message.From.ID)

	helpMsg := h.deps.Config.Messages.Help
	if h.deps.Config.Telegram.BotInfo.Username != "" {
		helpMsg = strings.ReplaceAll(helpMsg, "@botname", "@"+h.deps.Config.Telegram.BotInfo.Username)
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: helpMsg})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send help message", "error", err, "chat_id", update.Message.Chat.ID)
	} else {
		log.DebugContext(ctx, "Successfully sent help message", "chat_id", update.Message.Chat.ID)
	}
}
