// Package ai provides interfaces and implementations for interacting with different AI backends.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils" // Re-added for Sanitize
)

// OpenAIClient implements the AIClient interface using the OpenAI REST API.
type OpenAIClient struct {
	core       *AICore
	httpClient *http.Client
	model      string
	baseURL    string
	apiToken   string
}

// openAIMessage represents a single message in the OpenAI chat completion request.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatCompletionRequest represents the request payload for the OpenAI chat completions endpoint.
type openAIChatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float32         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"` // Added max_tokens
}

// openAIChatCompletionResponse represents the response payload from the OpenAI chat completions endpoint.
type openAIChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// newOpenAIClient creates a new OpenAI client using direct HTTP calls.
func newOpenAIClient(cfg *config.Config, core *AICore) (*OpenAIClient, error) {
	if core == nil {
		return nil, errors.New("AICore cannot be nil for OpenAIClient")
	}
	if cfg.AIToken == "" {
		return nil, errors.New("OpenAI API token (AIToken) is required")
	}

	return &OpenAIClient{
		core: core,
		httpClient: &http.Client{
			Timeout: core.Timeout(), // Use timeout from core
		},
		model:    cfg.AIModel,
		baseURL:  strings.TrimSuffix(cfg.AIBaseURL, "/"), // Ensure no trailing slash
		apiToken: cfg.AIToken,
	}, nil
}

// SetBotInfo updates the bot's identity information via the core.
func (c *OpenAIClient) SetBotInfo(info BotInfo) error {
	return c.core.SetBotInfo(info)
}

// GenerateResponse creates an AI-generated response using the OpenAI REST API.
func (c *OpenAIClient) GenerateResponse(ctx context.Context, request *Request) (string, error) {
	startTime := time.Now()
	if request == nil {
		return "", errors.New("nil request")
	}
	slog.Debug("generating OpenAI response", "user_id", request.UserID)

	if request.UserID <= 0 {
		return "", errors.New("invalid user ID")
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return "", errors.New("empty user message")
	}

	systemPrompt := c.core.CreateSystemPrompt(request.UserProfiles)

	// Prepare messages for OpenAI API format
	messages := []openAIMessage{{Role: "system", Content: systemPrompt}}
	for _, msg := range request.RecentMessages {
		role := "user"
		if msg.UserID == c.core.BotInfo().UserID {
			role = "assistant"
		}
		messages = append(messages, openAIMessage{Role: role, Content: formatMessage(msg)}) // formatMessage from core.go
	}
	currentMsgContent := formatMessage(&db.Message{ // formatMessage from core.go
		UserID:    request.UserID,
		Content:   request.Message,
		Timestamp: time.Now().UTC(),
	})
	messages = append(messages, openAIMessage{Role: "user", Content: currentMsgContent})

	// Prepare request payload
	apiRequest := openAIChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.core.Temperature(),
		MaxTokens:   2048, // Added a default max_tokens value
	}
	reqBodyBytes, err := json.Marshal(apiRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	// Create HTTP request
	// Corrected URL construction: Append only /chat/completions as baseURL likely includes /v1
	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OpenAI request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// OpenRouter headers removed as they didn't solve the 405 error.

	// Execute request
	apiStartTime := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	apiDuration := time.Since(apiStartTime)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			slog.Warn("failed to close OpenAI response body", "error", err)
		}
	}()

	// Read response body
	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenAI response body: %w", err)
	}

	// Check for non-200 status codes
	if httpResp.StatusCode != http.StatusOK {
		slog.Error("OpenAI API error", "status_code", httpResp.StatusCode, "response", string(respBodyBytes))
		// Try to parse error from response
		var errResp openAIChatCompletionResponse
		if json.Unmarshal(respBodyBytes, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("OpenAI API error (%d): %s (%s)", httpResp.StatusCode, errResp.Error.Message, errResp.Error.Type)
		}
		return "", fmt.Errorf("OpenAI API request failed with status code %d", httpResp.StatusCode)
	}

	// Parse successful response
	var apiResponse openAIChatCompletionResponse
	if err := json.Unmarshal(respBodyBytes, &apiResponse); err != nil {
		slog.Error("Failed to unmarshal OpenAI response", "error", err, "response", string(respBodyBytes))
		return "", fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if apiResponse.Error != nil {
		return "", fmt.Errorf("OpenAI API returned an error: %s (%s)", apiResponse.Error.Message, apiResponse.Error.Type)
	}

	if len(apiResponse.Choices) == 0 || apiResponse.Choices[0].Message.Content == "" {
		slog.Warn("OpenAI response contained no choices or empty content", "response_id", apiResponse.ID)
		return "", errors.New("no response choices returned from OpenAI")
	}

	rawResponse := apiResponse.Choices[0].Message.Content
	result, err := utils.Sanitize(rawResponse) // Use Sanitize from utils
	if err != nil {
		return "", fmt.Errorf("failed to sanitize OpenAI response: %w", err)
	}

	slog.Info("OpenAI response generated",
		"user_id", request.UserID,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"tokens", apiResponse.Usage.TotalTokens)

	return result, nil
}

// GenerateProfiles analyzes messages to create/update profiles using the OpenAI REST API.
func (c *OpenAIClient) GenerateProfiles(ctx context.Context, messages []*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	startTime := time.Now()
	slog.Debug("starting OpenAI profile generation", "messages", len(messages), "profiles", len(existingProfiles))

	if len(messages) == 0 {
		return nil, errors.New("no messages to analyze for profiles")
	}

	// Group messages by user (needed for parseProfileResponse)
	userMessages := make(map[int64][]*db.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	instruction := c.core.getProfileInstruction() // Get instruction from core

	// Prepare message content for the API call
	var msgBuilder strings.Builder
	if len(existingProfiles) > 0 {
		// Format existing profiles as JSON within the prompt message
		profileJSONBytes, err := json.MarshalIndent(map[string]interface{}{"users": existingProfiles}, "", "  ")
		if err != nil {
			slog.Warn("Failed to marshal existing profiles for prompt", "error", err)
			// Proceed without existing profiles in prompt if marshalling fails
		} else {
			msgBuilder.WriteString("## EXISTING USER PROFILES (JSON format)\n\n")
			msgBuilder.Write(profileJSONBytes)
			msgBuilder.WriteString("\n\n")
		}
	}
	msgBuilder.WriteString("## NEW GROUP CHAT MESSAGES\n\n")
	// Format new messages
	sortedUserIDs := make([]int64, 0, len(userMessages))
	for userID := range userMessages {
		sortedUserIDs = append(sortedUserIDs, userID)
	}
	sort.Slice(sortedUserIDs, func(i, j int) bool { return sortedUserIDs[i] < sortedUserIDs[j] })

	for _, userID := range sortedUserIDs {
		msgBuilder.WriteString(fmt.Sprintf("Messages from User %d:\n", userID))
		for _, msg := range userMessages[userID] {
			msgBuilder.WriteString(fmt.Sprintf("[%s] %s\n", msg.Timestamp.Format(time.RFC3339), msg.Content))
		}
		msgBuilder.WriteString("\n")
	}
	messageContent := msgBuilder.String()

	// Prepare API request payload
	apiMessages := []openAIMessage{
		{Role: "system", Content: instruction},
		{Role: "user", Content: messageContent},
	}
	apiRequest := openAIChatCompletionRequest{
		Model:       c.model, // Consider if a different model is needed for profiles
		Messages:    apiMessages,
		Temperature: c.core.Temperature(), // Use temperature from core
		MaxTokens:   4096,                 // Added a default max_tokens value (larger for profiles)
	}
	reqBodyBytes, err := json.Marshal(apiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI profile request: %w", err)
	}

	// Create and execute HTTP request
	// Corrected URL construction: Append only /chat/completions as baseURL likely includes /v1
	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI profile request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	httpReq.Header.Set("Accept", "application/json")

	// OpenRouter headers removed as they didn't solve the 405 error.

	apiStartTime := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	apiDuration := time.Since(apiStartTime)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI profile API: %w", err)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			slog.Warn("failed to close OpenAI profile response body", "error", err)
		}
	}()

	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI profile response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		slog.Error("OpenAI profile API error", "status_code", httpResp.StatusCode, "response", string(respBodyBytes))
		var errResp openAIChatCompletionResponse
		if json.Unmarshal(respBodyBytes, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("OpenAI profile API error (%d): %s (%s)", httpResp.StatusCode, errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("OpenAI profile API request failed with status code %d", httpResp.StatusCode)
	}

	var apiResponse openAIChatCompletionResponse
	if err := json.Unmarshal(respBodyBytes, &apiResponse); err != nil {
		slog.Error("Failed to unmarshal OpenAI profile response", "error", err, "response", string(respBodyBytes))
		return nil, fmt.Errorf("failed to parse OpenAI profile response: %w", err)
	}

	if apiResponse.Error != nil {
		return nil, fmt.Errorf("OpenAI profile API returned an error: %s (%s)", apiResponse.Error.Message, apiResponse.Error.Type)
	}

	if len(apiResponse.Choices) == 0 || apiResponse.Choices[0].Message.Content == "" {
		slog.Warn("OpenAI profile response contained no choices or empty content", "response_id", apiResponse.ID)
		return nil, errors.New("no profile response choices returned from OpenAI")
	}

	// Parse the response content using the core parser
	profiles, err := c.core.parseProfileResponse(apiResponse.Choices[0].Message.Content, userMessages, existingProfiles)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profiles from OpenAI response: %w", err)
	}

	slog.Info("OpenAI profile generation completed",
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"profile_count", len(profiles),
		"tokens", apiResponse.Usage.TotalTokens)

	return profiles, nil
}
