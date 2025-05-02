package handlers

import (
	"context"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// AdminOnly creates a middleware that restricts command access to users listed
// in the admin_users configuration. It protects sensitive administrative operations.
func AdminOnly(deps HandlerDeps) tgbot.Middleware {
	return func(next tgbot.HandlerFunc) tgbot.HandlerFunc {
		return func(ctx context.Context, bot *tgbot.Bot, update *models.Update) {
			if update.Message == nil || update.Message.From == nil {
				next(ctx, bot, update)
				return
			}

			userID := update.Message.From.ID
			adminID := deps.Config.Telegram.AdminUserID

			if userID != adminID {
				chatID := update.Message.Chat.ID
				log := deps.Logger.With("middleware", "AdminOnly")
				log.WarnContext(ctx, "Unauthorized access attempt", "user_id", userID, "chat_id", chatID)

				_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
					ChatID: chatID,
					Text:   deps.Config.Messages.ErrorUnauthorizedMsg,
				})
				if err != nil {
					log.ErrorContext(ctx, "Failed to send unauthorized message", "error", err, "chat_id", chatID)
				}
				return
			}

			next(ctx, bot, update)
		}
	}
}
