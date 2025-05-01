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
				// User is not the admin
				chatID := update.Message.Chat.ID
				// Get logger from deps
				log := deps.Logger.With("middleware", "AdminOnly")
				log.WarnContext(ctx, "Unauthorized access attempt", "user_id", userID, "chat_id", chatID)
				// Send unauthorized message using config
				_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
					ChatID: chatID,
					Text:   deps.Config.Messages.ErrorUnauthorizedMsg, // Updated field name
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

// Helper function isAdmin removed (duplicate of one in deps.go)

// Helper function sendReply removed (duplicate of one in deps.go)
