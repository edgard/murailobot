// Package ai provides interfaces and implementations for interacting with different AI backends.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
	"google.golang.org/genai"
)

// GeminiClient implements the AIClient interface using the Google Gemini Go SDK.
type GeminiClient struct {
	core                  *AICore
	client                *genai.Client
	modelName             string
	geminiSearchGrounding bool
}

// Map existing safety settings to the SDK's format using SDK types
var sdkSafetySettings = []*genai.SafetySetting{
	{
		Category:  genai.HarmCategoryHarassment,
		Threshold: genai.HarmBlockThresholdBlockNone,
	},
	{
		Category:  genai.HarmCategoryHateSpeech,
		Threshold: genai.HarmBlockThresholdBlockNone,
	},
	{
		Category:  genai.HarmCategorySexuallyExplicit,
		Threshold: genai.HarmBlockThresholdBlockNone,
	},
	{
		Category:  genai.HarmCategoryDangerousContent,
		Threshold: genai.HarmBlockThresholdBlockNone,
	},
}

const maxRetries = 3

// --- Client Implementation ---

func newGeminiClient(cfg *config.Config, core *AICore) (*GeminiClient, error) {
	if core == nil {
		return nil, errors.New("AICore cannot be nil for GeminiClient")
	}
	if cfg.AIToken == "" {
		return nil, errors.New("AI token (AIToken) is required for Gemini")
	}

	model := cfg.AIModel
	if model == "" || strings.HasPrefix(model, "gpt-") {
		slog.Warn("AIModel not specified or is OpenAI model; using default Gemini model", "model", model)
		return nil, errors.New("AIModel must be specified for Gemini")
	}

	ctx := context.Background()
	clientConfig := &genai.ClientConfig{
		APIKey:  cfg.AIToken,
		Backend: genai.BackendGeminiAPI,
	}

	// Use custom base URL if provided
	if cfg.AIBaseURL != "" && cfg.AIBaseURL != "https://api.openai.com/v1" {
		slog.Info("Using custom API base URL for Gemini", "base_url", cfg.AIBaseURL)
		// The GenAI client doesn't support custom URLs directly
		// This is a no-op for now, but kept for future compatibility
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	return &GeminiClient{
		core:                  core,
		client:                client,
		modelName:             model,
		geminiSearchGrounding: cfg.GeminiSearchGrounding,
	}, nil
}

func (c *GeminiClient) SetBotInfo(info BotInfo) error {
	return c.core.SetBotInfo(info)
}

func (c *GeminiClient) GenerateResponse(ctx context.Context, request *Request) (string, error) {
	startTime := time.Now()
	// Check for nil request before any dereference
	if request == nil {
		return "", errors.New("nil request")
	}
	slog.Debug("generating Gemini response", "user_id", request.UserID)
	// Apply timeout from core similar to OpenAIClient
	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.core.Timeout())
	defer cancel()
	// ...existing validation logic...
	if request.UserID <= 0 {
		return "", errors.New("invalid user ID")
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return "", errors.New("empty user message")
	}

	systemPrompt := c.core.CreateSystemPrompt(request.UserProfiles)
	// Build prompt only with user and assistant messages; system instructions via config
	var prompt []*genai.Content
	for _, msg := range request.RecentMessages {
		role := genai.RoleUser
		if msg.UserID == c.core.BotInfo().UserID {
			role = genai.RoleModel
		}
		prompt = append(prompt, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: formatMessage(msg)}},
		})
	}
	currentMsgContent := formatMessage(&db.Message{
		UserID:    request.UserID,
		Content:   request.Message,
		Timestamp: time.Now().UTC(),
	})
	prompt = append(prompt, &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: currentMsgContent}},
	})

	tools := []*genai.Tool{}
	if c.geminiSearchGrounding {
		tools = append(tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}

	var resp *genai.GenerateContentResponse
	var err error
	var apiDuration time.Duration
	for attempt := 1; attempt <= maxRetries; attempt++ {
		apiStartTime := time.Now()
		temperature := float32(c.core.Temperature())
		resp, err = c.client.Models.GenerateContent(ctxWithTimeout, c.modelName, prompt, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: systemPrompt}}},
			SafetySettings:    sdkSafetySettings,
			Temperature:       &temperature,
			Tools:             tools,
		})
		apiDuration = time.Since(apiStartTime)
		if err != nil {
			slog.Warn("Gemini API error, retrying", "error", err, "attempt", attempt)
			if attempt < maxRetries {
				continue
			}
			slog.Error("Gemini API error", "error", err)
			return "", fmt.Errorf("failed to call Gemini API after %d attempts: %w", attempt, err)
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
			slog.Warn("Gemini response contained no choices or empty content, retrying", "attempt", attempt)
			if attempt < maxRetries {
				continue
			}
			return "", errors.New("no response choices returned from Gemini after retries")
		}
		break
	}
	candidate := resp.Candidates[0]

	var responseBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		responseBuilder.WriteString(part.Text)
	}
	result, err := utils.Sanitize(responseBuilder.String())
	if err != nil {
		return "", fmt.Errorf("failed to sanitize Gemini response: %w", err)
	}

	slog.Info("Gemini response generated",
		"user_id", request.UserID,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"tokens", candidate.TokenCount)

	return result, nil
}

func (c *GeminiClient) GenerateProfiles(ctx context.Context, messages []*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	startTime := time.Now()
	slog.Debug("starting Gemini profile generation using SDK", "messages", len(messages), "profiles", len(existingProfiles))

	if len(messages) == 0 {
		return nil, errors.New("no messages to analyze for profiles")
	}

	// Apply timeout from core similar to OpenAIClient
	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.core.Timeout())
	defer cancel()
	// Group messages by non-bot users (needed for parseProfileResponse)
	userMessages := make(map[int64][]*db.Message)
	for _, msg := range messages {
		if msg.UserID == c.core.BotInfo().UserID {
			continue
		}
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	if len(userMessages) == 0 {
		slog.Info("No non-bot messages found for profile generation")
		return existingProfiles, nil
	}

	var prompt []*genai.Content
	instruction := c.core.getProfileInstruction()

	if len(existingProfiles) > 0 {
		profileJSONBytes, err := json.MarshalIndent(map[string]interface{}{"users": existingProfiles}, "", "  ")
		if err != nil {
			slog.Warn("Failed to marshal existing profiles for Gemini prompt", "error", err)
		} else {
			prompt = append(prompt, &genai.Content{
				Role:  genai.RoleUser,
				Parts: []*genai.Part{{Text: "## EXISTING USER PROFILES (JSON format)\n\n" + string(profileJSONBytes)}}},
			)
		}
	}

	prompt = append(prompt, &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: "## NEW GROUP CHAT MESSAGES"}},
	})

	sortedUserIDs := make([]int64, 0, len(userMessages))
	for userID := range userMessages {
		sortedUserIDs = append(sortedUserIDs, userID)
	}
	sort.Slice(sortedUserIDs, func(i, j int) bool { return sortedUserIDs[i] < sortedUserIDs[j] })

	for _, userID := range sortedUserIDs {
		var userMsgBuilder strings.Builder
		userMsgBuilder.WriteString(fmt.Sprintf("Messages from User %d:\n", userID))
		sort.SliceStable(userMessages[userID], func(i, j int) bool {
			return userMessages[userID][i].Timestamp.Before(userMessages[userID][j].Timestamp)
		})
		for _, msg := range userMessages[userID] {
			userMsgBuilder.WriteString(fmt.Sprintf("[%s] %s\n", msg.Timestamp.Format(time.RFC3339), msg.Content))
		}
		prompt = append(prompt, &genai.Content{
			Role:  genai.RoleUser,
			Parts: []*genai.Part{{Text: userMsgBuilder.String()}},
		})
	}

	var profileResp *genai.GenerateContentResponse
	var perr error
	var profileAPIDuration time.Duration
	for attempt := 1; attempt <= maxRetries; attempt++ {
		apiStart := time.Now()
		temperature := float32(c.core.Temperature())
		profileResp, perr = c.client.Models.GenerateContent(ctxWithTimeout, c.modelName, prompt, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: instruction}}},
			SafetySettings:    sdkSafetySettings,
			Temperature:       &temperature,
		})
		profileAPIDuration = time.Since(apiStart)
		if perr != nil {
			slog.Warn("Gemini profile API error, retrying", "error", perr, "attempt", attempt)
			if attempt < maxRetries {
				continue
			}
			slog.Error("Gemini profile API error", "error", perr)
			return nil, fmt.Errorf("failed to call Gemini profile API after %d attempts: %w", attempt, perr)
		}
		if len(profileResp.Candidates) == 0 || profileResp.Candidates[0].Content == nil || len(profileResp.Candidates[0].Content.Parts) == 0 {
			slog.Warn("Gemini profile response contained no choices or empty content, retrying", "attempt", attempt)
			if attempt < maxRetries {
				continue
			}
			return nil, errors.New("no profile response choices returned from Gemini after retries")
		}
		break
	}
	candidate := profileResp.Candidates[0]

	var responseBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		responseBuilder.WriteString(part.Text)
	}
	rawJSONResponse := extractTextFromPotentialMarkdown(responseBuilder.String())

	profiles, err := c.core.parseProfileResponse(rawJSONResponse, userMessages, existingProfiles)
	if err != nil {
		slog.Error("Failed to parse profiles from Gemini SDK response", "error", err, "raw_response", rawJSONResponse)
		return nil, fmt.Errorf("failed to parse profiles from Gemini SDK response: %w", err)
	}

	tokenCount := int(candidate.TokenCount)
	slog.Info("Gemini profile generation completed",
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", profileAPIDuration.Milliseconds(),
		"profile_count", len(profiles),
		"tokens", tokenCount)

	return profiles, nil
}

func extractTextFromPotentialMarkdown(input string) string {
	startIndex := strings.Index(input, "```json")
	if startIndex == -1 {
		startIndex = strings.Index(input, "```")
		if startIndex == -1 {
			return strings.TrimSpace(input)
		}
		startIndex += 3
	} else {
		startIndex += len("```json")
	}

	endIndex := strings.Index(input[startIndex:], "```")
	if endIndex == -1 {
		return strings.TrimSpace(input[startIndex:])
	}
	endIndex += startIndex
	extracted := input[startIndex:endIndex]
	return strings.TrimSpace(extracted)
}
