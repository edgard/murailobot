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
	// The response is sanitized and validated before being returned.
	// Returns:
	// - ErrEmptyUserMessage if the message is empty
	// - ErrNoChoices if the API returns no completion choices
	// - ErrEmptyResponse if the API returns an empty response
	Generate(userID int64, userMsg string) (string, error)

	// Analysis methods

	// GenerateGroupAnalysis creates a behavioral analysis.
	// Returns:
	// - ErrNoMessages if messages slice is empty
	// - ErrJSONUnmarshal if the API response cannot be parsed
	GenerateGroupAnalysis(messages []db.GroupMessage) (map[int64]*db.UserAnalysis, error)
}

// Client implements the Service interface using OpenAI's API.
// It manages API communication, response generation, and conversation history.
type Client struct {
	// OpenAI client configuration
	aiClient    *openai.Client // Configured OpenAI API client
	model       string         // Model identifier (e.g., "gpt-3.5-turbo")
	temperature float32        // Response randomness (0.0-1.0)

	// Behavioral configuration
	instruction string        // System instruction for chat context
	timeout     time.Duration // Maximum time for API operations

	// Dependencies
	db Database // Database for conversation history
}

// Database defines the required database operations for AI functionality.
// It provides access to conversation history.
type Database interface {
	// GetRecent retrieves recent chat history.
	// It returns up to 'limit' entries ordered by timestamp descending.
	GetRecent(limit int) ([]db.ChatHistory, error)
}

// completionRequest encapsulates parameters for an AI completion API call.
// It includes all necessary information for generating and tracking responses.
type completionRequest struct {
	messages   []openai.ChatCompletionMessage // Ordered conversation messages
	userID     int64                          // User identifier for logging
	attemptNum *uint                          // Current retry attempt number
}
