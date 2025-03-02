package ai

import (
	"context"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/sashabaranov/go-openai"
)

// Constants for magic numbers.
const (
	httpClientTimeoutDivisor = 2
	minHTTPClientTimeout     = 30 * time.Second
	retryMaxAttempts         = 3
	initialBackoffDuration   = 100 * time.Millisecond
	chatHistoryTimeout       = 5 * time.Second
	recentHistoryCount       = 10
	messagesSliceCapacity    = 21
)

// Non-retryable OpenAI API errors.
var invalidRequestErrors = []string{
	"invalid_request_error",
	"context_length_exceeded",
	"rate_limit_exceeded",
	"invalid_api_key",
	"organization_not_found",
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
	openaiClient *openai.Client
	model        string
	temperature  float32
	instruction  string
	db           db.Database
	timeout      time.Duration
}

// Service defines AI service interface.
type Service interface {
	Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error)
}
