package telegram

import (
	"fmt"
	"log/slog"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func (b *Bot) isAuthorized(userID int64) bool {
	// Admin is always authorized
	if userID == b.cfg.AdminUID {
		return true
	}

	// Check blocked users first (using map for O(1) lookup)
	blockedMap := make(map[int64]bool, len(b.security.BlockedUserIDs))
	for _, id := range b.security.BlockedUserIDs {
		blockedMap[id] = true
	}
	if blockedMap[userID] {
		return false
	}

	// If allowed list is empty, all non-blocked users are allowed
	if len(b.security.AllowedUserIDs) == 0 {
		return true
	}

	// Check allowed users (using map for O(1) lookup)
	allowedMap := make(map[int64]bool, len(b.security.AllowedUserIDs))
	for _, id := range b.security.AllowedUserIDs {
		allowedMap[id] = true
	}
	return allowedMap[userID]
}

func (b *Bot) sendUnauthorizedMessage(bot *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	slog.Warn("unauthorized access attempt", "user_id", userID)
	_, err := ctx.EffectiveMessage.Reply(bot, b.cfg.Messages.NotAuthorized, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to send unauthorized message: %v", ErrBot, err)
	}
	return ErrUnauthorized
}

func (b *Bot) sanitizeMessage(text string) string {
	return b.ai.SanitizeResponse(text)
}
