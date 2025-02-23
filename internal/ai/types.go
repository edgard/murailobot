package ai

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Service provides AI chat functionality with message history.
type Service interface {
	Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error)
	Sanitize(response string) string
}

// CompletionService abstracts OpenAI API operations for testing and provider switching.
type CompletionService interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}
