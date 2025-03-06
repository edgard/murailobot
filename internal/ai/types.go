// Package ai provides integration with OpenAI's API for chat completion
// with conversation history and automatic retries.
package ai

import (
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

const (
	initialBackoffDuration = 1 * time.Second
	messagesPerHistory     = 2
	messagesSliceCapacity  = 20
	recentHistoryCount     = 10
	retryMaxAttempts       = 3
)

var (
	ErrNilConfig        = errors.New("config is nil")
	ErrEmptyResponse    = errors.New("empty response received")
	ErrEmptyUserMessage = errors.New("user message is empty")
	ErrNoChoices        = errors.New("no response choices available")
)

// Service defines the interface for generating AI responses.
type Service interface {
	// Generate creates an AI response for a given user message, incorporating
	// conversation history and user context.
	Generate(userID int64, userName string, userMsg string) (string, error)
}

// Client implements the Service interface using OpenAI's API.
type Client struct {
	aiClient    *openai.Client
	model       string
	temperature float32
	instruction string
	timeout     time.Duration
	db          db.Database
}
