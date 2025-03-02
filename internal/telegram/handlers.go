package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleStart processes the /start command, sending a welcome message to new users.
// This is typically the first interaction users have with the bot.
//
// Parameters:
//   - ctx: Context for timeout/cancellation (unused but kept for consistency)
//   - msg: The message containing the command
//
// Returns an error if the message is nil or sending the welcome message fails.
func (b *Bot) handleStart(_ context.Context, msg *tgbotapi.Message) error {
	if msg == nil {
		return ErrNilMessage
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.cfg.Messages.Welcome)

	return b.sendMessage(reply, "failed to send welcome message")
}

// handleMessage processes the /mrl command, which generates AI responses to user messages.
// It manages the complete flow of:
//  1. Extracting and validating the user's message
//  2. Showing typing indicator during processing
//  3. Generating AI response
//  4. Saving conversation history
//  5. Sending response back to user
//
// The function handles various error conditions and provides appropriate user feedback.
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

// handleReset processes the /mrl_reset command, which clears all conversation history.
// This is an admin-only command that requires authorization checking.
//
// The function:
//  1. Validates the message and user authorization
//  2. Deletes all conversation history
//  3. Sends confirmation or error message to user
//
// Returns an error if:
//   - Message is nil
//   - User is not authorized
//   - Database operation fails
//   - Sending response fails
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

// isUserAuthorized checks if a user has admin privileges by comparing
// their user ID with the configured admin ID.
func (b *Bot) isUserAuthorized(userID int64) bool {
	return userID == b.cfg.AdminID
}

// sendMessage is a helper function that sends a message via the Telegram API
// and wraps any errors with a descriptive message.
//
// Parameters:
//   - msg: The message configuration to send
//   - errMsg: Description to prepend to any error that occurs
//
// Returns an error if the API call fails.
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig, errMsg string) error {
	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	return nil
}
