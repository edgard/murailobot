package ai

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Service defines the required AI operations
type Service interface {
	GenerateResponse(ctx context.Context, userID int64, userName string, userMsg string) (string, error)
	SanitizeResponse(response string) string
}

// CompletionService defines the interface for OpenAI API operations
type CompletionService interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}
