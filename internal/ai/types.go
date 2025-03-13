package ai

import (
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

// History and message capacity constants.
const (
	recentHistoryCount    = 10 // Number of recent messages to include in context
	messagesSliceCapacity = 20 // Initial capacity for messages slice
	messagesPerHistory    = 2  // Number of messages per history entry (user + bot)
)

// Message context constants.
const (
	extraMessageSlots = 2 // Additional slots for system and user context
)

// Fixed part of the profile instruction that must not be configurable.
const profileInstructionFixed = `
### Bot Influence Awareness
- DO NOT attribute traits based on topics introduced by the bot
- If the bot mentions a topic and the user merely responds, this is not evidence of a personal trait
- Only identify traits from topics and interests the user has independently demonstrated
- Ignore creative embellishments that might have been added by the bot in previous responses

## OUTPUT FORMAT [VERY CRITICAL]
Return ONLY a JSON object, no additional text, with this structure:
{
  "users": {
    "[user_id]": {
      "display_names": "Comma-separated list of names/nicknames",
      "origin_location": "Where the user is from",
      "current_location": "Where the user currently lives",
      "age_range": "Approximate age range (20s, 30s, etc.)",
      "traits": "Comma-separated list of personality traits and characteristics"
    }
  }
}
`

// Service defines the interface for AI operations.
// It provides methods for generating AI responses and analyzing user behavior.
type Service interface {
	// Message generation methods

	// Generate creates an AI response for a user message.
	// If userProfiles is provided, it will be used to personalize the response
	// with awareness of all users in the group.
	// The response is sanitized and validated before being returned.
	Generate(userID int64, userMsg string, userProfiles map[int64]*db.UserProfile) (string, error)

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
	db database // Database for conversation history

	// Bot information
	botUID         int64  // Bot's Telegram User ID
	botUsername    string // Bot's Telegram username
	botDisplayName string // Bot's Telegram display name
}

// database defines the required database operations for AI functionality.
// It provides access to conversation history and user profiles.
type database interface {
	// GetRecent retrieves recent chat history.
	// It returns up to 'limit' entries ordered by timestamp descending.
	GetRecent(limit int) ([]db.ChatHistory, error)

	// GetUserProfile retrieves a user's profile by user ID.
	GetUserProfile(userID int64) (*db.UserProfile, error)
}

// completionRequest encapsulates parameters for an AI completion API call.
type completionRequest struct {
	messages []openai.ChatCompletionMessage // Ordered conversation messages
	userID   int64                          // User identifier for logging
}
