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
				errMsg := deps.Config.Messages.NotAuthorized
				_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
					ChatID: update.Message.Chat.ID,
					Text:   errMsg,
				})
				if err != nil {
					deps.Logger.ErrorContext(ctx, "Failed to send 'Not Authorized' message", "error", err, "user_id", userID, "chat_id", update.Message.Chat.ID)
				}
				// Stop processing this update for subsequent handlers/middleware
				deps.Logger.WarnContext(ctx, "Unauthorized access attempt blocked", "user_id", userID, "admin_id", adminID)
				return // Stop processing for this handler chain
			}

			// User is the admin, proceed to the next handler
			next(ctx, bot, update)
		}
	}
}

// Helper function isAdmin removed (duplicate of one in deps.go)

// Helper function sendReply removed (duplicate of one in deps.go)
