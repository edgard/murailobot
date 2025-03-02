package openai

import (
	"context"
	"errors"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

// Service defines the OpenAI response generation interface.
type Service interface {
	Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error)
}

// Config defines OpenAI settings.
type Config struct {
	Token       string        `yaml:"token"`
	BaseURL     string        `yaml:"base_url"`
	Model       string        `yaml:"model"`
	Temperature float32       `yaml:"temperature"`
	Instruction string        `yaml:"instruction"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Client represents the OpenAI client.
type Client struct {
	openAIClient *openai.Client
	model        string
	temperature  float32
	instruction  string
	db           db.Database
	timeout      time.Duration
}

// Operation timeouts and retry settings.
const (
	ChatHistoryTimeout       = 5 * time.Second
	HTTPClientTimeoutDivisor = 4
	InitialBackoffDuration   = 1 * time.Second
	MessagesPerHistory       = 2
	MessagesSliceCapacity    = 20
	MinHTTPClientTimeout     = 10 * time.Second
	RecentHistoryCount       = 10
	RetryMaxAttempts         = 3
)

// Known non-retryable OpenAI API error types.
var InvalidRequestErrors = []string{
	"invalid_request_error",
	"context_length_exceeded",
	"rate_limit_exceeded",
	"invalid_api_key",
	"organization_not_found",
}

// Error definitions.
var (
	ErrNilConfig        = errors.New("config is nil")
	ErrEmptyResponse    = errors.New("empty response received")
	ErrEmptyUserMessage = errors.New("user message is empty")
	ErrNoChoices        = errors.New("no response choices available")
)
