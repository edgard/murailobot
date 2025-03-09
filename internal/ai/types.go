package ai

import (
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

const (
	recentHistoryCount     = 10
	messagesSliceCapacity  = 20
	messagesPerHistory     = 2
	retryMaxAttempts       = 3
	initialBackoffDuration = 2 * time.Second
	hoursInDay             = 24

	// Additional message slots for system and user context.
	extraMessageSlots = 2
)

var (
	ErrNilConfig        = errors.New("nil config provided")
	ErrEmptyUserMessage = errors.New("empty user message")
	ErrNoChoices        = errors.New("no choices in API response")
	ErrEmptyResponse    = errors.New("empty response from API")
	ErrNoMessages       = errors.New("no messages to analyze")
	ErrNoUserMessages   = errors.New("target user has no messages")
	ErrUserNotFound     = errors.New("user analysis not found in response")
	ErrJSONMarshal      = errors.New("failed to marshal JSON data")
	ErrJSONUnmarshal    = errors.New("failed to unmarshal JSON data")
)

// Service defines the interface for AI operations.
type Service interface {
	// Generate creates an AI response for a user message.
	Generate(userID int64, userMsg string) (string, error)
	// GenerateGroupAnalysis creates a behavioral analysis for all users in the provided messages.
	GenerateGroupAnalysis(messages []db.GroupMessage) (map[int64]*db.UserAnalysis, error)
}

// Client implements the Service interface using OpenAI's API.
type Client struct {
	aiClient    *openai.Client
	model       string
	temperature float32
	instruction string
	timeout     time.Duration
	db          Database
}

// Database defines the required database operations for AI functionality.
type Database interface {
	// GetRecent retrieves recent chat history.
	GetRecent(limit int) ([]db.ChatHistory, error)
}

// completionRequest holds parameters for an AI completion request.
type completionRequest struct {
	messages   []openai.ChatCompletionMessage
	userID     int64
	attemptNum *uint
}
