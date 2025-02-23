package ai

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Service provides AI-powered chat functionality with message history
// and response sanitization capabilities.
type Service interface {
	// GenerateResponse creates an AI response for the given user message.
	// It uses chat history for context and ensures the response is properly
	// formatted for the messaging platform.
	GenerateResponse(ctx context.Context, userID int64, userName string, userMsg string) (string, error)

	// SanitizeResponse ensures the AI response text is compatible with
	// the messaging platform's formatting requirements.
	SanitizeResponse(response string) string
}

// CompletionService abstracts the OpenAI API operations to allow
// for easier testing and potential provider switching.
type CompletionService interface {
	// CreateChatCompletion sends a chat completion request to the AI provider
	// and returns the generated response.
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}
