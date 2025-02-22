package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/ai"
)

func (b *Bot) handleStart(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return fmt.Errorf("%w: message is nil", ErrBot)
	}

	slog.Info("received /start command",
		"user_id", msg.From.Id,
		"chat_id", ctx.EffectiveChat.Id,
		"username", msg.From.Username,
	)

	userID := msg.From.Id
	if !b.isAuthorized(userID) {
		return b.sendUnauthorizedMessage(bot, ctx, userID)
	}

	msgCtx, cancel := context.WithTimeout(context.Background(), b.cfg.Polling.RequestTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		welcomeMsg := b.sanitizeMessage(b.cfg.Messages.Welcome)
		_, err := msg.Reply(bot, welcomeMsg, nil)
		done <- err
	}()

	select {
	case <-msgCtx.Done():
		return fmt.Errorf("%w: message send timeout", ErrBot)
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w: failed to send welcome message: %v", ErrBot, err)
		}
	}
	return nil
}

func (b *Bot) handleChatMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return fmt.Errorf("%w: message is nil", ErrBot)
	}

	userID := msg.From.Id
	slog.Info("received /mrl command",
		"user_id", userID,
		"chat_id", ctx.EffectiveChat.Id,
		"username", msg.From.Username,
		"has_text", msg.Text != "/mrl",
		"text_length", len(msg.Text),
	)

	if !b.isAuthorized(userID) {
		return b.sendUnauthorizedMessage(bot, ctx, userID)
	}

	// Create a parent context for the entire operation
	opCtx, cancel := context.WithTimeout(context.Background(), b.cfg.AIRequestTimeout)
	defer cancel()

	// Start typing indicator
	typingCtx, cancelTyping := context.WithCancel(opCtx)
	go b.sendContinuousTyping(typingCtx, bot, ctx.EffectiveChat.Id)
	defer cancelTyping() // Ensure typing indicator stops when function returns

	// Validate and sanitize input
	text := strings.TrimSpace(msg.Text)
	if text == "/mrl" {
		msgCtx, cancel := context.WithTimeout(opCtx, b.cfg.Polling.RequestTimeout)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, err := msg.Reply(bot, b.cfg.Messages.ProvideMessage, nil)
			done <- err
		}()

		select {
		case <-msgCtx.Done():
			return fmt.Errorf("%w: message send timeout", ErrBot)
		case err := <-done:
			if err != nil {
				return fmt.Errorf("%w: failed to send prompt message: %v", ErrBot, err)
			}
		}
		return nil
	}

	if len(text) > b.security.MaxMessageLength {
		// Create context with timeout for message sending
		msgCtx, cancel := context.WithTimeout(opCtx, b.cfg.Polling.RequestTimeout)
		defer cancel()

		errMsg := b.sanitizeMessage(fmt.Sprintf(b.cfg.Messages.MessageTooLong, b.security.MaxMessageLength))

		// Use done channel to handle timeout
		done := make(chan error, 1)
		go func() {
			_, err := msg.Reply(bot, errMsg, nil)
			done <- err
		}()

		// Wait for either timeout or completion
		select {
		case <-msgCtx.Done():
			return fmt.Errorf("%w: message send timeout", ErrBot)
		case err := <-done:
			if err != nil {
				return fmt.Errorf("%w: failed to send length error message: %v", ErrBot, err)
			}
		}
		return nil
	}

	// Create dedicated context for AI operation
	aiCtx, aiCancel := context.WithTimeout(context.Background(), b.cfg.AIRequestTimeout)
	defer aiCancel()

	// Generate AI response with dedicated context
	username := strings.TrimSpace(msg.From.Username)
	if username == "" {
		username = "Unknown"
	}
	response, err := b.ai.GenerateResponse(aiCtx, userID, username, text)
	if err != nil {
		var errMsg string
		if errors.Is(err, ai.ErrAI) {
			errMsg = b.cfg.Messages.AIError
		} else if errors.Is(err, context.DeadlineExceeded) {
			errMsg = "Request timed out. Please try again."
		} else {
			errMsg = b.cfg.Messages.GeneralError
		}

		slog.Error("failed to generate AI response",
			"error", err,
			"user_id", msg.From.Id,
			"chat_id", ctx.EffectiveChat.Id,
			"message_length", len(text),
			"error_type", func() string {
				switch {
				case errors.Is(err, ai.ErrAI):
					return "AI error"
				case errors.Is(err, context.DeadlineExceeded):
					return "timeout"
				default:
					return "general error"
				}
			}(),
		)

		errMsg = b.sanitizeMessage(errMsg)

		msgCtx, cancel := context.WithTimeout(opCtx, b.cfg.Polling.RequestTimeout)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, replyErr := msg.Reply(bot, errMsg, nil)
			done <- replyErr
		}()

		select {
		case <-msgCtx.Done():
			return fmt.Errorf("%w: error message send timeout", ErrBot)
		case replyErr := <-done:
			if replyErr != nil {
				return fmt.Errorf("%w: failed to send error message: %v", ErrBot, replyErr)
			}
		}
		return err
	}

	// Validate response length
	if len(response) > b.security.MaxMessageLength {
		response = response[:b.security.MaxMessageLength-3] + "..."
	}

	// Save chat history with timeout context
	dbCtx, cancel := context.WithTimeout(opCtx, b.cfg.DBOperationTimeout)
	defer cancel()

	if err := b.db.SaveChatInteraction(dbCtx, msg.From.Id, username, text, response); err != nil {
		slog.Error("failed to save chat history",
			"error", err,
			"user_id", msg.From.Id,
			"chat_id", msg.Chat.Id,
		)
		// Continue execution - saving history is not critical
	}

	response = b.sanitizeMessage(response)
	_, err = msg.Reply(bot, response, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to send response: %v", ErrBot, err)
	}
	return nil
}

func (b *Bot) handleChatHistoryReset(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return fmt.Errorf("%w: message is nil", ErrBot)
	}

	userID := msg.From.Id
	slog.Info("received /mrl_reset command",
		"user_id", userID,
		"chat_id", ctx.EffectiveChat.Id,
		"username", msg.From.Username,
		"is_admin", userID == b.cfg.AdminUID,
	)

	if userID != b.cfg.AdminUID {
		return b.sendUnauthorizedMessage(bot, ctx, userID)
	}

	// Create a parent context for the operation
	opCtx, cancel := context.WithTimeout(context.Background(), b.cfg.DBOperationTimeout)
	defer cancel()

	// Start typing indicator
	typingCtx, cancelTyping := context.WithCancel(opCtx)
	go b.sendContinuousTyping(typingCtx, bot, ctx.EffectiveChat.Id)
	defer cancelTyping()

	// Clear chat history with timeout context
	if err := b.db.DeleteAllChatHistory(opCtx); err != nil {
		slog.Error("failed to reset chat history",
			"error", err,
			"user_id", userID,
			"chat_id", ctx.EffectiveChat.Id,
		)
		_, replyErr := msg.Reply(bot, b.cfg.Messages.GeneralError, nil)
		if replyErr != nil {
			return fmt.Errorf("%w: failed to send error message: %v", ErrBot, replyErr)
		}
		return fmt.Errorf("%w: failed to reset chat history: %v", ErrBot, err)
	}

	resetMsg := b.sanitizeMessage(b.cfg.Messages.HistoryReset)
	_, err := msg.Reply(bot, resetMsg, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to send reset confirmation: %v", ErrBot, err)
	}

	slog.Info("chat history reset successful",
		"user_id", userID,
		"chat_id", ctx.EffectiveChat.Id,
	)
	return nil
}
