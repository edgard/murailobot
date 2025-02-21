package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// typingManager handles typing indicator operations
type typingManager struct {
	cfg *Config
}

// newTypingManager creates a new typing manager
func newTypingManager(cfg *Config) *typingManager {
	return &typingManager{
		cfg: cfg,
	}
}

func (t *typingManager) sendContinuousTyping(ctx context.Context, bot *gotgbot.Bot, chatID int64) {
	ticker := time.NewTicker(t.cfg.TypingInterval)
	defer ticker.Stop()

	// Send initial typing action
	if err := t.sendTypingAction(bot, chatID); err != nil {
		slog.Error("failed to send initial typing action",
			"error", err,
			"chat_id", chatID,
		)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.sendTypingAction(bot, chatID); err != nil {
				if ctx.Err() != nil {
					return // Context cancelled, exit normally
				}
				// Log non-critical typing failures
				slog.Debug("typing action failed",
					"error", err,
					"chat_id", chatID,
				)
			}
		}
	}
}

func (t *typingManager) sendTypingAction(bot *gotgbot.Bot, chatID int64) error {
	_, err := bot.SendChatAction(chatID, "typing", &gotgbot.SendChatActionOpts{
		RequestOpts: &gotgbot.RequestOpts{
			Timeout: t.cfg.TypingActionTimeout,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send typing action: %w", err)
	}
	return nil
}
