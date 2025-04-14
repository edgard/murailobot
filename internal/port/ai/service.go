// Package ai defines the interface for AI service interactions.
package ai

import (
	"context"

	"github.com/edgard/murailobot/internal/domain/model"
)

// BotInfo contains information about the bot for AI context.
type BotInfo struct {
	UserID      int64
	Username    string
	DisplayName string
}

// Request represents a request for AI processing.
type Request struct {
	UserID         int64
	Message        string
	RecentMessages []*model.Message
	UserProfiles   map[int64]*model.UserProfile
}

// Service defines the interface for AI operations.
type Service interface {
	// SetBotInfo configures bot information for AI context
	SetBotInfo(info BotInfo) error

	// GenerateResponse generates a response to a user message
	GenerateResponse(ctx context.Context, request *Request) (string, error)

	// GenerateProfiles analyzes messages to generate user profiles
	GenerateProfiles(ctx context.Context, messages []*model.Message, existingProfiles map[int64]*model.UserProfile) (map[int64]*model.UserProfile, error)

	// CreateSystemPrompt generates the system prompt for AI interactions
	CreateSystemPrompt(userProfiles map[int64]*model.UserProfile) string
}
