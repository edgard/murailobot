// Package ai provides artificial intelligence capabilities for MurailoBot,
// handling message response generation and user profile analysis using
// OpenAI's API or compatible services.
package ai

import (
	"github.com/edgard/murailobot/internal/db"
)

// Request contains all the information needed to generate an AI response.
// It includes the user's message, conversation context, and user profiles
// to help create more personalized and contextually appropriate responses.
type Request struct {
	UserID         int64
	Message        string
	RecentMessages []*db.Message
	UserProfiles   map[int64]*db.UserProfile
}

// BotInfo contains identification information about the bot itself.
// This information is used to properly handle the bot's own messages
// in conversation history and to create the bot's profile.
type BotInfo struct {
	UserID      int64
	Username    string
	DisplayName string
}

// ChatMessage represents a single message in the chat completion API request/response
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider represents provider-specific settings for the API request
type Provider struct {
	RequireParameters bool `json:"require_parameters"`
}

// SchemaType represents a JSON schema structure
type SchemaType struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties"`
	Required             []string            `json:"required"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

// Property represents a JSON schema property with optional nested schema
type Property struct {
	Type                 string              `json:"type,omitempty"`
	Description          string              `json:"description,omitempty"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties interface{}         `json:"additionalProperties,omitempty"`
	Items                *Property           `json:"items,omitempty"`
}

// JSONSchema represents the structure of the JSON schema for response validation
type JSONSchema struct {
	Name   string     `json:"name"`
	Strict bool       `json:"strict"`
	Schema SchemaType `json:"schema"`
}

// ResponseFormat represents the desired format for the API response
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// ChatCompletionRequest represents the request structure for chat completion API
type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage   `json:"messages"`
	Temperature    float32         `json:"temperature"`
	Provider       *Provider       `json:"provider,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// ChatCompletionResponse represents the response structure from chat completion API
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}
