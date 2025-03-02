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
		return errors.New("message is nil")
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Welcome)
	_, err := b.api.Send(reply)
	if err != nil {
		return fmt.Errorf("failed to send welcome message: %w", err)
	}

	return nil
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return errors.New("message is nil")
	}

	text := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/mrl"))
	if text == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Provide)
		_, err := b.api.Send(reply)
		if err != nil {
			return fmt.Errorf("failed to send prompt message: %w", err)
		}

		return nil
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

	response, err := b.ai.Generate(ctx, msg.From.ID, userName, text)
	if err != nil {
		slog.Error("failed to generate AI response",
			"error", err,
			"user_id", msg.From.ID,
			"chat_id", msg.Chat.ID)

		errMsg := b.cfg.Messages.GeneralError
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg = b.cfg.Messages.Timeout
		}

		reply := tgbotapi.NewMessage(msg.Chat.ID, errMsg)
		if _, replyErr := b.api.Send(reply); replyErr != nil {
			return fmt.Errorf("failed to send error message (%w) after AI error: %w", replyErr, err)
		}

		return fmt.Errorf("AI generation failed: %w", err)
	}

	// Save history but continue if save fails
	if err := b.db.Save(ctx, msg.From.ID, userName, text, response); err != nil {
		slog.Warn("failed to save chat history",
			"error", err,
			"user_id", msg.From.ID)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	if _, err := b.api.Send(reply); err != nil {
		return fmt.Errorf("failed to send AI response: %w", err)
	}

	return nil
}

func (b *Bot) handleReset(ctx context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return errors.New("message is nil")
	}

	userID := msg.From.ID
	if !b.isUserAuthorized(userID) {
		return b.sendUnauthorizedMsg(msg)
	}

	slog.Info("resetting chat history", "user_id", userID)

	if err := b.db.DeleteAll(ctx); err != nil {
		slog.Error("failed to reset chat history",
			"error", err,
			"user_id", userID)

		reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.GeneralError)
		if _, replyErr := b.api.Send(reply); replyErr != nil {
			return fmt.Errorf("failed to send error message (%w) after history reset error: %w", replyErr, err)
		}

		return fmt.Errorf("history reset failed: %w", err)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.HistoryReset)
	if _, err := b.api.Send(reply); err != nil {
		return fmt.Errorf("failed to send history reset confirmation: %w", err)
	}

	return nil
}

func (b *Bot) isUserAuthorized(userID int64) bool {
	return userID == b.cfg.AdminID
}

func (b *Bot) sendUnauthorizedMsg(msg *tgbotapi.Message) error {
	slog.Warn("unauthorized access attempt",
		"user_id", msg.From.ID,
		"action", "reset_history")

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Unauthorized)
	_, err := b.api.Send(reply)
	if err != nil {
		return fmt.Errorf("failed to send unauthorized message: %w", err)
	}

	return nil
}
