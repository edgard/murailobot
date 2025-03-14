package ai

import (
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/text"
	"github.com/sashabaranov/go-openai"
)

// Message capacity constants.
const (
	messagesSliceCapacity = 20 // Initial capacity for messages slice
)

// Message context constants.
const (
	extraMessageSlots = 2 // Additional slots for system and user context
)

// Service defines the interface for AI operations.
// It provides methods for generating AI responses and analyzing user behavior.
type Service interface {
	// Message generation methods

	// Generate creates an AI response for a user message.
	// If recentMessages is provided, it will be used as context for the response.
	// If userProfiles is provided, it will be used to personalize the response
	// with awareness of all users in the group.
	// The response is sanitized and validated before being returned.
	Generate(userID int64, userMsg string, recentMessages []db.GroupMessage, userProfiles map[int64]*db.UserProfile) (string, error)

	// Analysis methods

	// GenerateUserProfiles creates or updates user profiles based on message analysis.
	// It considers existing profiles when performing the analysis.
	GenerateUserProfiles(messages []db.GroupMessage, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error)

	// Configuration methods

	// SetBotInfo sets the bot's Telegram User ID, username, and display name for profile handling.
	// This information will be used to add special handling for the bot in user profiles.
	SetBotInfo(uid int64, username string, displayName string) error
}

// client implements the Service interface using OpenAI's API.
// It manages API communication, response generation, and conversation history.
type client struct {
	// OpenAI client configuration
	aiClient    *openai.Client // Configured OpenAI API client
	model       string         // Model identifier (e.g., "gpt-3.5-turbo")
	temperature float32        // Response randomness (0.0-1.0)

	// Behavioral configuration
	instruction        string        // System instruction for chat context
	profileInstruction string        // System instruction for profile generation
	timeout            time.Duration // Maximum time for API operations

	// Dependencies
	db            db.Database         // Database for conversation history
	dynamicWindow *text.DynamicWindow // Dynamic context window manager

	// Bot information
	botUID         int64  // Bot's Telegram User ID
	botUsername    string // Bot's Telegram username
	botDisplayName string // Bot's Telegram display name
}

// completionRequest encapsulates parameters for an AI completion API call.
type completionRequest struct {
	messages []openai.ChatCompletionMessage // Ordered conversation messages
	userID   int64                          // User identifier for logging
}
