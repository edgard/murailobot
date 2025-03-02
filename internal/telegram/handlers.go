package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleStart(_ context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Welcome)

	return b.sendMessage(reply, "failed to send welcome message")
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	text := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/mrl"))
	if text == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Provide)

		return b.sendMessage(reply, "failed to send prompt message")
	}

	userName := ""
	if msg.From.UserName != "" {
		userName = "@" + msg.From.UserName
	} else if msg.From.FirstName != "" {
		userName = msg.From.FirstName
	}

	displayName := "unknown"
	if userName != "" {
		displayName = userName
	}

	slog.Info("processing chat request",
		"user_id", msg.From.ID,
		"username", displayName,
		"message_length", len(text))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go b.SendContinuousTyping(ctx, msg.Chat.ID)

	response, err := b.openAI.Generate(ctx, msg.From.ID, userName, text)
	if err != nil {
		slog.Error("failed to generate OpenAI response",
			"error", err,
			"user_id", msg.From.ID,
			"chat_id", msg.Chat.ID)

		errMsg := b.cfg.Messages.GeneralError
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg = b.cfg.Messages.Timeout
		}

		reply := tgbotapi.NewMessage(msg.Chat.ID, errMsg)
		if err := b.sendMessage(reply, "failed to send error message"); err != nil {
			slog.Error("failed to send error message to user",
				"error", err,
				"user_id", msg.From.ID)
		}

		return fmt.Errorf("OpenAI generation failed: %w", err)
	}

	// Save history but continue if save fails
	if err := b.db.Save(ctx, msg.From.ID, userName, text, response); err != nil {
		slog.Warn("failed to save chat history",
			"error", err,
			"user_id", msg.From.ID)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if err := b.sendMessage(reply, "failed to send OpenAI response"); err != nil {
		slog.Error("failed to send OpenAI response",
			"error", err,
			"user_id", msg.From.ID)

		return fmt.Errorf("failed to send OpenAI response: %w", err)
	}

	return nil
}

func (b *Bot) handleReset(ctx context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		slog.Warn("unauthorized access attempt",
			"user_id", msg.From.ID,
			"action", "reset_history")

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
		if err := b.sendMessage(reply, "failed to send unauthorized message"); err != nil {
			slog.Error("failed to send unauthorized message",
				"error", err,
				"user_id", msg.From.ID)
		}

		return ErrUnauthorized
	}

	slog.Info("resetting chat history", "user_id", userID)

	if err := b.db.DeleteAll(ctx); err != nil {
		slog.Error("failed to reset chat history",
			"error", err,
			"user_id", userID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if err := b.sendMessage(reply, "failed to send error message"); err != nil {
			slog.Error("failed to send error message to user",
				"error", err,
				"user_id", userID)
		}

		return fmt.Errorf("history reset failed: %w", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.HistoryReset)
	if err := b.sendMessage(reply, "failed to send reset confirmation"); err != nil {
		slog.Error("failed to send reset confirmation",
			"error", err,
			"user_id", userID)

		return fmt.Errorf("history reset succeeded but failed to confirm: %w", err)
	}

	return nil
}

func (b *Bot) isUserAuthorized(userID int64) bool {
	return userID == b.cfg.AdminID
}

// Helper function to reduce code duplication.
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig, errMsg string) error {
	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	return nil
}
