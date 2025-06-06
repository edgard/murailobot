package handlers

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewStartHandler creates a handler for the /start command that welcomes users
// and provides initial guidance on how to interact with the bot.
func NewStartHandler(deps HandlerDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		log := deps.Logger.With("handler", "start")

		if update.Message == nil || update.Message.From == nil {
			log.DebugContext(ctx, "Ignoring update with nil message or sender")
			return
		}

		chatID := update.Message.Chat.ID
		userID := update.Message.From.ID
		username := update.Message.From.Username

		log.InfoContext(ctx, "/start command received", "chat_id", chatID, "user_id", userID, "username", username)

		welcome := deps.Config.Messages.StartWelcomeMsg

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: welcome})
		if err != nil {
			log.ErrorContext(ctx, "Failed to send welcome message", "error", err, "chat_id", chatID)
		}
	}
}
