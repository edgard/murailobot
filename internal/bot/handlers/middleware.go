// Package handlers contains Telegram bot command and message handlers,
// along with their registration logic and middleware.
package handlers

import (
	"context"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// AdminOnly creates a middleware that checks if the message sender is the configured admin user.
// If not, it sends a "Not Authorized" message and stops processing by returning early.
func AdminOnly(deps HandlerDeps) tgbot.Middleware {
	return func(next tgbot.HandlerFunc) tgbot.HandlerFunc {
		return func(ctx context.Context, bot *tgbot.Bot, update *models.Update) {
			// Ensure it's a message update and From is not nil
			if update.Message == nil || update.Message.From == nil {
				// Not a message or no sender info, let it pass or handle as needed
				// For command handlers, this usually won't happen, but good practice.
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
				return // Stop processing
			}

			// User is the admin, proceed to the next handler
			next(ctx, bot, update)
		}
	}
}
