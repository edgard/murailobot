// Package openai provides integration with OpenAI's API for generating chat responses.
// It handles message history management, retry logic, and error handling while
// maintaining conversation context.
package openai

import (
	"context"
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

// Service defines the OpenAI response generation interface.
// Implementations must handle conversation context, API communication,
// and error handling while generating responses.
type Service interface {
	// Generate creates an AI response for a user message.
	// It includes user context and handles conversation history.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation
	//   - userID: Telegram user identifier
	//   - userName: User's display name (optional)
	//   - userMsg: The message to generate a response for
	//
	// Returns the generated response or an error if generation fails.
	Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error)
}

// Config defines OpenAI settings used to configure the client.
// These settings control API behavior, model selection, and response characteristics.
type Config struct {
	Token       string        // OpenAI API authentication token
	BaseURL     string        // API endpoint URL
	Model       string        // Model identifier (e.g., "gpt-4")
	Temperature float32       // Response randomness (0.0-2.0)
	Instruction string        // System message defining assistant behavior
	Timeout     time.Duration // API call timeout duration
}

// Client represents the OpenAI client implementation.
// It maintains configuration and handles API communication while
// implementing the Service interface.
type Client struct {
	openAIClient *openai.Client // Underlying OpenAI API client
	model        string         // Current model identifier
	temperature  float32        // Response temperature setting
	instruction  string         // System instruction
	db           db.Database    // Database for conversation history
	timeout      time.Duration  // API timeout setting
}

// Operation timeouts and retry settings define various timing and capacity constants
// used throughout the package.
const (
	chatHistoryTimeout       = 5 * time.Second  // Timeout for database history retrieval
	httpClientTimeoutDivisor = 4                // Divides API timeout for HTTP client
	initialBackoffDuration   = 1 * time.Second  // Starting retry delay
	messagesPerHistory       = 2                // Messages per history entry (user + assistant)
	messagesSliceCapacity    = 20               // Initial capacity for message slice
	minHTTPClientTimeout     = 10 * time.Second // Minimum HTTP client timeout
	recentHistoryCount       = 10               // Number of recent messages to include
	retryMaxAttempts         = 3                // Maximum retry attempts for API calls
)

// invalidRequestErrors lists known non-retryable OpenAI API error types.
// When these errors occur, the operation will fail immediately without retrying.
var invalidRequestErrors = []string{
	"invalid_request_error",   // General API request validation failure
	"context_length_exceeded", // Too many tokens in the request
	"rate_limit_exceeded",     // API rate limit reached
	"invalid_api_key",         // Authentication failure
	"organization_not_found",  // Invalid organization ID
}

// Error definitions for common error conditions.
var (
	ErrNilConfig        = errors.New("config is nil")                 // Configuration not provided
	ErrEmptyResponse    = errors.New("empty response received")       // API returned empty content
	ErrEmptyUserMessage = errors.New("user message is empty")         // No user message provided
	ErrNoChoices        = errors.New("no response choices available") // API returned no choices
)
