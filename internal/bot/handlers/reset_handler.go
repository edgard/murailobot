package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewResetHandler creates a handler for the /mrl_reset command that deletes
// all messages and user profiles from the database, providing a clean slate.
func NewResetHandler(deps HandlerDeps) bot.HandlerFunc {
	return resetHandler{deps}.handle
}

type resetHandler struct {
	deps HandlerDeps
}

func (h resetHandler) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	log := h.deps.Logger.With("handler", "reset")

	if update.Message == nil || update.Message.From == nil {
		log.ErrorContext(ctx, "Reset handler called with nil Message or From", "update_id", update.ID)
		return
	}

	chatID := update.Message.Chat.ID
	log.InfoContext(ctx, "Admin requested data reset", "chat_id", chatID, "user_id", update.Message.From.ID)

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := h.deps.Store.DeleteAllMessagesAndProfiles(timeoutCtx)

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

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.deps.Config.Messages.ResetConfirmMsg,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to send reset confirmation message", "error", err, "chat_id", chatID)
	}
}
