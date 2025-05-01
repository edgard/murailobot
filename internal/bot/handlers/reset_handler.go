// Package handlers contains Telegram bot command and message handlers,
// along with their registration logic.
package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewResetHandler returns a handler for the /mrl_reset command.
func NewResetHandler(deps HandlerDeps) bot.HandlerFunc {
	return resetHandler{deps}.Handle
}

// resetHandler holds dependencies for the data reset command.
type resetHandler struct {
	deps HandlerDeps
}

// Handle processes the /mrl_reset command, which deletes all stored messages and user profiles.
// It uses a timeout context to prevent the operation from running indefinitely and performs
// the deletion within a single database transaction for atomicity.
// Requires admin privileges (enforced by middleware).
func (h resetHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "reset")

	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Reset handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	log.InfoContext(ctx, "Admin requested data reset", "chat_id", chatID, "user_id", update.Message.From.ID)

	// Create a timeout context to prevent the operation from running too long
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel() // Ensure resources are cleaned up

	// Use the atomic delete operation that ensures both tables are deleted in a transaction
	err := h.deps.Store.DeleteAllMessagesAndProfiles(timeoutCtx)

	// Handle timeout errors specifically
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		log.WarnContext(ctx, "Reset operation timed out or was cancelled", "error", err, "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ResetTimeoutMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send reset timeout message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Handle other database errors
	if err != nil {
		log.ErrorContext(ctx, "Failed to reset data", "error", err, "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ResetErrorMsg,
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send reset error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	log.InfoContext(ctx, "Successfully deleted all messages and user profiles", "chat_id", chatID)

	// Send confirmation message
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.deps.Config.Messages.ResetConfirmMsg,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send reset confirmation message", "error", err, "chat_id", chatID)
	}
}
