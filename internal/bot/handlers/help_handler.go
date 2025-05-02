package handlers

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewHelpHandler creates a handler for the /help command that provides
// usage information to users about available bot commands.
func NewHelpHandler(deps HandlerDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		log := deps.Logger.With("handler", "help")

		if update.Message == nil || update.Message.From == nil {
			log.DebugContext(ctx, "Ignoring update with nil message or sender")
			return
		}

		chatID := update.Message.Chat.ID
		username := update.Message.From.Username
		userID := update.Message.From.ID

		log.InfoContext(ctx, "/help command received", "chat_id", chatID, "user_id", userID, "username", username)

		helpMsg := deps.Config.Messages.HelpMsg

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: helpMsg})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send help message", "error", err, "chat_id", chatID)
		}
	}
}
