package ai

import (
	"errors"
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

// Retry and timeout constants.
const (
	retryMaxAttempts       = 3               // Maximum number of API call retries
	initialBackoffDuration = 2 * time.Second // Initial delay between retries
)

// Message context constants.
const (
	extraMessageSlots = 2 // Additional slots for system and user context
)

// Error definitions for the AI package.
var (
	// Configuration errors.
	ErrNilConfig = errors.New("nil config provided")

	// Input validation errors.
	ErrEmptyUserMessage = errors.New("empty user message")
	ErrNoMessages       = errors.New("no messages to analyze")

	// API response errors.
	ErrNoChoices     = errors.New("no choices in API response")
	ErrEmptyResponse = errors.New("empty response from API")
	ErrJSONUnmarshal = errors.New("failed to unmarshal JSON response")
)

// Service defines the interface for AI operations.
// It provides methods for generating AI responses and analyzing user behavior.
type Service interface {
	// Message generation methods

	// Generate creates an AI response for a user message.
	// If userProfiles is provided, it will be used to personalize the response
	// with awareness of all users in the group.
	// The response is sanitized and validated before being returned.
	// Returns:
	// - ErrEmptyUserMessage if the message is empty
	// - ErrNoChoices if the API returns no completion choices
	// - ErrEmptyResponse if the API returns an empty response
	Generate(userID int64, userMsg string, userProfiles map[int64]*db.UserProfile) (string, error)

	// Analysis methods

	// GenerateUserProfiles creates or updates user profiles based on message analysis.
	// It considers existing profiles when performing the analysis.
	// Returns:
	// - ErrNoMessages if messages slice is empty
	// - ErrJSONUnmarshal if the API response cannot be parsed
	GenerateUserProfiles(messages []db.GroupMessage, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error)

	// Configuration methods

	// SetBotInfo sets the bot's Telegram User ID, username, and display name for profile handling.
	// This information will be used to add special handling for the bot in user profiles.
	SetBotInfo(uid int64, username string, displayName string)
}

// client implements the Service interface using OpenAI's API.
// It manages API communication, response generation, and conversation history.
type client struct {
	// OpenAI client configuration
	aiClient    *openai.Client // Configured OpenAI API client
	model       string         // Model identifier (e.g., "gpt-3.5-turbo")
	temperature float32        // Response randomness (0.0-1.0)

	// Behavioral configuration
	instruction string        // System instruction for chat context
	timeout     time.Duration // Maximum time for API operations

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
// It includes all necessary information for generating and tracking responses.
type completionRequest struct {
	messages   []openai.ChatCompletionMessage // Ordered conversation messages
	userID     int64                          // User identifier for logging
	attemptNum *uint                          // Current retry attempt number
}
