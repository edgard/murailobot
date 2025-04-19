// Package ai provides interfaces and implementations for interacting with different AI backends.
package ai

import (
	"context"

	"github.com/edgard/murailobot/internal/db"
)

// AIClient defines the interface for interacting with an AI backend.
// It provides methods for generating text responses and user profiles.
type AIClient interface {
	// GenerateResponse generates a text response based on the provided request context.
	GenerateResponse(ctx context.Context, request *Request) (string, error)

	// GenerateProfiles analyzes messages to create or update user profiles.
	GenerateProfiles(ctx context.Context, messages []*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error)

	// SetBotInfo sets the bot's identity information.
	SetBotInfo(info BotInfo) error
}
