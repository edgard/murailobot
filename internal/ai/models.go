// Package ai provides artificial intelligence capabilities for MurailoBot,
// handling message response generation and user profile analysis using
// OpenAI's API or compatible services.
package ai

import (
	"github.com/edgard/murailobot/internal/db"
)

// Request contains all the information needed to generate an AI response.
// It includes the user's message, conversation context, and user profiles
// to help create more personalized and contextually appropriate responses.
type Request struct {
	UserID         int64
	Message        string
	RecentMessages []*db.Message
	UserProfiles   map[int64]*db.UserProfile
}

// BotInfo contains identification information about the bot itself.
// This information is used to properly handle the bot's own messages
// in conversation history and to create the bot's profile.
type BotInfo struct {
	UserID      int64
	Username    string
	DisplayName string
}
