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

type resetHandler struct {
	deps HandlerDeps
}

func (h resetHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "reset")
	// Ensure Message and From are not nil (should be guaranteed by middleware/handler registration)
	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Reset handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	log.InfoContext(ctx, "Admin requested data reset", "chat_id", chatID, "user_id", update.Message.From.ID)

	// Create a timeout context to prevent the operation from running too long
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel() // Ensure resources are cleaned up

	// Use the new atomic delete operation that ensures both tables are deleted in a transaction
	err := h.deps.Store.DeleteAllMessagesAndProfiles(timeoutCtx)

	// Handle timeout errors specifically
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		log.WarnContext(ctx, "Reset operation timed out or was cancelled", "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ResetTimeoutMsg, // Use config message
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send timeout message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	// Handle other errors
	if err != nil {
		log.ErrorContext(ctx, "Failed to reset data", "error", err, "chat_id", chatID)
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.deps.Config.Messages.ResetErrorMsg, // Updated field name
		})
		if sendErr != nil {
			log.ErrorContext(ctx, "Failed to send error message", "error", sendErr, "chat_id", chatID)
		}
		return
	}

	log.InfoContext(ctx, "Successfully deleted all messages and user profiles in a single transaction", "chat_id", chatID)

	// Send confirmation
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.deps.Config.Messages.ResetConfirmMsg, // Updated field name
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send reset confirmation message", "error", err, "chat_id", chatID)
	}
}

// Deprecated: original newResetHandler kept for reference, does not affect registry.
// func newResetHandler(deps HandlerDeps) bot.HandlerFunc { ... }
