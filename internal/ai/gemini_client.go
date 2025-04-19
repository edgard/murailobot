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
	"github.com/edgard/murailobot/internal/utils"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"
	defaultGeminiModel   = "gemini-1.5-flash-latest"
)

// GeminiClient implements the AIClient interface using the Google Gemini REST API.
type GeminiClient struct {
	core                  *AICore
	httpClient            *http.Client
	model                 string
	baseURL               string
	apiToken              string
	geminiSearchGrounding bool
}

// --- Gemini API Structures ---

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type geminiGenerationConfig struct {
	Temperature     float32  `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	TopP            float32  `json:"topP,omitempty"`
	TopK            int      `json:"topK,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// --- Tool Structure ---
type geminiGoogleSearchTool struct {
	// Empty struct signifies enabling default Google Search
}

type geminiTool struct {
	GoogleSearch *geminiGoogleSearchTool `json:"google_search,omitempty"`
}

// --- Main Request/Response Structures ---

type geminiChatCompletionRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Tools             []geminiTool           `json:"tools,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings    []geminiSafetySetting  `json:"safetySettings,omitempty"`
}

var defaultSafetySettings = []geminiSafetySetting{
	{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
	{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
	{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
	{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
	// SafetyRatings, CitationMetadata, GroundingMetadata can be added if needed
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiChatCompletionResponse struct {
	Candidates     []geminiCandidate   `json:"candidates"`
	UsageMetadata  geminiUsageMetadata `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason   string               `json:"blockReason,omitempty"`
		SafetyRatings []geminiSafetyRating `json:"safetyRatings,omitempty"`
	} `json:"promptFeedback,omitempty"`
}

type geminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked,omitempty"` // Indicates if this specific rating caused a block
}

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
		model = defaultGeminiModel
		slog.Warn("AIModel not specified or is OpenAI model; using default Gemini model", "model", model)
	}

	return &GeminiClient{
		core: core,
		httpClient: &http.Client{
			Timeout: core.Timeout(),
		},
		model:                 model,
		baseURL:               defaultGeminiBaseURL,
		apiToken:              cfg.AIToken,
		geminiSearchGrounding: cfg.GeminiSearchGrounding,
	}, nil
}

func (c *GeminiClient) SetBotInfo(info BotInfo) error {
	return c.core.SetBotInfo(info)
}

func (c *GeminiClient) buildGeminiURL() string {
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiToken)
}

func (c *GeminiClient) GenerateResponse(ctx context.Context, request *Request) (string, error) {
	startTime := time.Now()
	if request == nil {
		return "", errors.New("nil request")
	}
	slog.Debug("generating Gemini response", "user_id", request.UserID, "grounding", c.geminiSearchGrounding)

	if request.UserID <= 0 {
		return "", errors.New("invalid user ID")
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return "", errors.New("empty user message")
	}

	systemPrompt := c.core.CreateSystemPrompt(request.UserProfiles)

	contents := []geminiContent{}
	isUserTurn := true

	if len(request.RecentMessages) == 0 {
		fullFirstMessage := formatMessage(&db.Message{
			UserID:    request.UserID,
			Content:   request.Message,
			Timestamp: time.Now().UTC(),
		})
		contents = append(contents, geminiContent{
			Role:  "user",
			Parts: []geminiPart{{Text: fullFirstMessage}},
		})
	} else {
		for _, msg := range request.RecentMessages {
			role := "user"
			if msg.UserID == c.core.BotInfo().UserID {
				role = "model"
			}

			messageText := formatMessage(msg)

			if len(contents) > 0 && contents[len(contents)-1].Role == role {
				slog.Warn("Skipping message due to consecutive same roles for Gemini", "user_id", msg.UserID, "role", role)
				continue
			}

			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: messageText}},
			})
			isUserTurn = (role == "model")
		}

		currentMsgContent := formatMessage(&db.Message{
			UserID:    request.UserID,
			Content:   request.Message,
			Timestamp: time.Now().UTC(),
		})
		if !isUserTurn {
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: currentMsgContent}},
			})
		} else {
			slog.Warn("Appending current user message after another user message for Gemini", "user_id", request.UserID)
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: currentMsgContent}},
			})
		}
	}

	apiRequest := geminiChatCompletionRequest{
		Contents: contents,
		GenerationConfig: geminiGenerationConfig{
			Temperature: c.core.Temperature(),
		},
		SafetySettings: defaultSafetySettings,
	}

	if systemPrompt != "" {
		apiRequest.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	if c.geminiSearchGrounding {
		apiRequest.Tools = []geminiTool{
			{
				GoogleSearch: &geminiGoogleSearchTool{},
			},
		}
	}

	reqBodyBytes, err := json.Marshal(apiRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	slog.Debug("Gemini request body", "body", string(reqBodyBytes))

	url := c.buildGeminiURL()
	maxRetries := 1 // Allow one retry specifically for SAFETY blocks
	var lastErr error
	var apiDuration time.Duration

	for attempt := 0; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBodyBytes))
		if err != nil {
			return "", fmt.Errorf("failed to create Gemini request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

		apiStartTime := time.Now()
		httpResp, err := c.httpClient.Do(httpReq)
		apiDuration = time.Since(apiStartTime) // Capture duration for the last successful/failed attempt

		if err != nil {
			lastErr = fmt.Errorf("failed to call Gemini API (attempt %d): %w", attempt, err)
			// Don't retry on connection errors, fail fast
			return "", lastErr
		}

		respBodyBytes, err := io.ReadAll(httpResp.Body)
		if err := httpResp.Body.Close(); err != nil {
			slog.Warn("failed to close Gemini response body", "attempt", attempt, "error", err)
		}
		if err != nil {
			lastErr = fmt.Errorf("failed to read Gemini response body (attempt %d): %w", attempt, err)
			// Don't retry on read errors, fail fast
			return "", lastErr
		}

		if httpResp.StatusCode != http.StatusOK {
			slog.Error("Gemini API error", "attempt", attempt, "status_code", httpResp.StatusCode, "response", string(respBodyBytes))
			lastErr = fmt.Errorf("gemini API request failed (attempt %d) with status code %d: %s", attempt, httpResp.StatusCode, string(respBodyBytes))
			// Don't retry on non-200 status codes, fail fast
			return "", lastErr
		}

		var apiResponse geminiChatCompletionResponse
		if err := json.Unmarshal(respBodyBytes, &apiResponse); err != nil {
			slog.Error("Failed to unmarshal Gemini response", "attempt", attempt, "error", err, "response", string(respBodyBytes))
			lastErr = fmt.Errorf("failed to parse Gemini response (attempt %d): %w", attempt, err)
			// Don't retry on unmarshal errors, fail fast
			return "", lastErr
		}

		if apiResponse.PromptFeedback != nil {
			slog.Warn("Gemini response included prompt feedback", "attempt", attempt,
				"block_reason", apiResponse.PromptFeedback.BlockReason,
				"safety_ratings", fmt.Sprintf("%+v", apiResponse.PromptFeedback.SafetyRatings))
		}

		finishReason := ""
		if len(apiResponse.Candidates) > 0 {
			finishReason = apiResponse.Candidates[0].FinishReason
		}

		// Check if content is valid
		if len(apiResponse.Candidates) > 0 && len(apiResponse.Candidates[0].Content.Parts) > 0 && apiResponse.Candidates[0].Content.Parts[0].Text != "" {
			// Success case
			var responseBuilder strings.Builder
			for _, part := range apiResponse.Candidates[0].Content.Parts {
				responseBuilder.WriteString(part.Text)
			}
			rawResponse := responseBuilder.String()

			result, err := utils.Sanitize(rawResponse)
			if err != nil {
				return "", fmt.Errorf("failed to sanitize Gemini response: %w", err) // No retry on sanitize error
			}

			slog.Info("Gemini response generated",
				"user_id", request.UserID,
				"duration_ms", time.Since(startTime).Milliseconds(),
				"api_ms", apiDuration.Milliseconds(),
				"tokens", apiResponse.UsageMetadata.TotalTokenCount,
				"attempts", attempt+1)

			return result, nil
		}

		// --- Handle empty/blocked content ---
		if finishReason == "" && len(apiResponse.Candidates) > 0 {
			finishReason = "UNKNOWN" // Assign if candidate exists but reason is missing
		} else if len(apiResponse.Candidates) == 0 {
			finishReason = "NO_CANDIDATES"
		}

		logFields := []interface{}{"attempt", attempt, "finish_reason", finishReason}
		if apiResponse.PromptFeedback != nil {
			logFields = append(logFields, "block_reason", apiResponse.PromptFeedback.BlockReason)
			logFields = append(logFields, "safety_ratings", fmt.Sprintf("%+v", apiResponse.PromptFeedback.SafetyRatings))
		} else if finishReason == "SAFETY" {
			logFields = append(logFields, "feedback_details", "missing_from_api")
		}
		slog.Warn("Gemini response contained no candidates or empty content", logFields...)

		// Decide whether to retry or fail
		if finishReason == "SAFETY" && attempt < maxRetries {
			slog.Info("Retrying Gemini request due to SAFETY block", "attempt", attempt+1)
			lastErr = fmt.Errorf("gemini response blocked due to safety settings (attempt %d)", attempt) // Store error in case retry fails
			time.Sleep(500 * time.Millisecond)                                                           // Small delay before retry
			continue                                                                                     // Go to next attempt
		}

		// --- Failure cases (no retry or final attempt failed) ---
		// --- Failure cases (no retry or final attempt failed) ---
		switch finishReason {
		case "SAFETY":
			blockReasonDetail := "safety settings"
			if apiResponse.PromptFeedback != nil && apiResponse.PromptFeedback.BlockReason != "" {
				blockReasonDetail = fmt.Sprintf("safety settings (reason: %s)", apiResponse.PromptFeedback.BlockReason)
			}
			lastErr = fmt.Errorf("gemini response blocked due to %s after %d attempts", blockReasonDetail, attempt+1)
		case "RECITATION":
			lastErr = errors.New("gemini response blocked due to potential recitation")
		default:
			lastErr = fmt.Errorf("no valid response choices returned from Gemini after %d attempts (finishReason: %s)", attempt+1, finishReason)
		}
		break // Exit loop on non-retryable error or final attempt failure
	}

	// If loop finished without returning success, return the last recorded error
	return "", lastErr
}

func (c *GeminiClient) GenerateProfiles(ctx context.Context, messages []*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	startTime := time.Now()
	slog.Debug("starting Gemini profile generation", "messages", len(messages), "profiles", len(existingProfiles))

	if len(messages) == 0 {
		return nil, errors.New("no messages to analyze for profiles")
	}

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

	instruction := c.core.getProfileInstruction()

	var msgBuilder strings.Builder

	if len(existingProfiles) > 0 {
		profileJSONBytes, err := json.MarshalIndent(map[string]interface{}{"users": existingProfiles}, "", "  ")
		if err != nil {
			slog.Warn("Failed to marshal existing profiles for Gemini prompt", "error", err)
		} else {
			msgBuilder.WriteString("## EXISTING USER PROFILES (JSON format)\n\n")
			msgBuilder.Write(profileJSONBytes)
			msgBuilder.WriteString("\n\n")
		}
	}
	msgBuilder.WriteString("## NEW GROUP CHAT MESSAGES\n\n")
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

	apiRequest := geminiChatCompletionRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: messageContent}}},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature: c.core.Temperature(),
		},
		SafetySettings: defaultSafetySettings,
	}

	if instruction != "" {
		apiRequest.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: instruction}},
		}
	}

	reqBodyBytes, err := json.Marshal(apiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini profile request: %w", err)
	}

	slog.Debug("Gemini profile request body", "body", string(reqBodyBytes))

	url := c.buildGeminiURL()
	maxRetries := 2 // Allow two retries specifically for SAFETY/RECITATION blocks
	var lastErr error
	var apiDuration time.Duration
	var finalTokenCount int

	for attempt := 0; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini profile request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

		apiStartTime := time.Now()
		httpResp, err := c.httpClient.Do(httpReq)
		apiDuration = time.Since(apiStartTime) // Capture duration for the last successful/failed attempt

		if err != nil {
			lastErr = fmt.Errorf("failed to call Gemini profile API (attempt %d): %w", attempt, err)
			return nil, lastErr // Fail fast on connection errors
		}

		respBodyBytes, err := io.ReadAll(httpResp.Body)
		if err := httpResp.Body.Close(); err != nil {
			slog.Warn("failed to close Gemini profile response body", "attempt", attempt, "error", err)
		}
		if err != nil {
			lastErr = fmt.Errorf("failed to read Gemini profile response body (attempt %d): %w", attempt, err)
			return nil, lastErr // Fail fast on read errors
		}

		if httpResp.StatusCode != http.StatusOK {
			slog.Error("Gemini profile API error", "attempt", attempt, "status_code", httpResp.StatusCode, "response", string(respBodyBytes))
			lastErr = fmt.Errorf("gemini profile API request failed (attempt %d) with status code %d: %s", attempt, httpResp.StatusCode, string(respBodyBytes))
			return nil, lastErr // Fail fast on non-200 status
		}

		var apiResponse geminiChatCompletionResponse
		if err := json.Unmarshal(respBodyBytes, &apiResponse); err != nil {
			// Try extracting from markdown before failing completely on unmarshal error
			slog.Error("Failed to unmarshal Gemini profile response", "attempt", attempt, "error", err, "response", string(respBodyBytes))
			rawJSONResponse := extractTextFromPotentialMarkdown(string(respBodyBytes))
			if rawJSONResponse != "" {
				slog.Warn("Attempting to parse extracted text as JSON for profiles", "attempt", attempt, "extracted_text", rawJSONResponse)
				profiles, parseErr := c.core.parseProfileResponse(rawJSONResponse, userMessages, existingProfiles)
				if parseErr == nil {
					slog.Info("Successfully parsed profiles from extracted text after initial unmarshal failure")
					logProfileGenerationSuccess(startTime, apiDuration, profiles, 0) // Token count unknown here
					return profiles, nil
				}
				slog.Error("Failed to parse extracted text as JSON", "attempt", attempt, "parse_error", parseErr)
				lastErr = fmt.Errorf("failed to parse extracted JSON from Gemini profile response (attempt %d): %w", attempt, parseErr)
			} else {
				lastErr = fmt.Errorf("failed to parse Gemini profile response (attempt %d): %w", attempt, err)
			}
			// Don't retry on unmarshal/parse errors
			break
		}

		// --- Process valid JSON response ---
		finalTokenCount = apiResponse.UsageMetadata.TotalTokenCount // Store token count from the successful attempt

		if apiResponse.PromptFeedback != nil {
			slog.Warn("Gemini profile response included prompt feedback", "attempt", attempt,
				"block_reason", apiResponse.PromptFeedback.BlockReason,
				"safety_ratings", fmt.Sprintf("%+v", apiResponse.PromptFeedback.SafetyRatings))
		}

		finishReason := ""
		if len(apiResponse.Candidates) > 0 {
			finishReason = apiResponse.Candidates[0].FinishReason
		}

		// Check if content is valid
		if len(apiResponse.Candidates) > 0 && len(apiResponse.Candidates[0].Content.Parts) > 0 && apiResponse.Candidates[0].Content.Parts[0].Text != "" {
			// Success case
			var responseBuilder strings.Builder
			for _, part := range apiResponse.Candidates[0].Content.Parts {
				responseBuilder.WriteString(part.Text)
			}
			rawJSONResponse := responseBuilder.String()
			rawJSONResponse = extractTextFromPotentialMarkdown(rawJSONResponse)

			profiles, err := c.core.parseProfileResponse(rawJSONResponse, userMessages, existingProfiles)
			if err != nil {
				slog.Error("Failed to parse profiles from Gemini response", "attempt", attempt, "error", err, "raw_response", rawJSONResponse)
				lastErr = fmt.Errorf("failed to parse profiles from Gemini response (attempt %d): %w", attempt, err)
				break // Don't retry on parse error
			}

			logProfileGenerationSuccess(startTime, apiDuration, profiles, finalTokenCount)
			return profiles, nil
		}

		// --- Handle empty/blocked content ---
		if finishReason == "" && len(apiResponse.Candidates) > 0 {
			finishReason = "UNKNOWN"
		} else if len(apiResponse.Candidates) == 0 {
			finishReason = "NO_CANDIDATES"
		}

		logFields := []interface{}{"attempt", attempt, "finish_reason", finishReason}
		if apiResponse.PromptFeedback != nil {
			logFields = append(logFields, "block_reason", apiResponse.PromptFeedback.BlockReason)
			logFields = append(logFields, "safety_ratings", fmt.Sprintf("%+v", apiResponse.PromptFeedback.SafetyRatings))
		} else if finishReason == "SAFETY" || finishReason == "RECITATION" {
			logFields = append(logFields, "feedback_details", "missing_from_api")
		}
		slog.Warn("Gemini profile response contained no candidates or empty content", logFields...)

		// Decide whether to retry or fail
		if (finishReason == "SAFETY" || finishReason == "RECITATION") && attempt < maxRetries {
			slog.Info("Retrying Gemini profile request due to block", "attempt", attempt+1, "reason", finishReason)
			lastErr = fmt.Errorf("gemini profile generation blocked due to %s (attempt %d)", strings.ToLower(finishReason), attempt)
			time.Sleep(500 * time.Millisecond) // Small delay
			continue                           // Go to next attempt
		}

		// --- Failure cases (no retry or final attempt failed) ---
		if finishReason == "SAFETY" || finishReason == "RECITATION" {
			blockReasonDetail := strings.ToLower(finishReason)
			if apiResponse.PromptFeedback != nil && apiResponse.PromptFeedback.BlockReason != "" {
				blockReasonDetail = fmt.Sprintf("%s (reason: %s)", blockReasonDetail, apiResponse.PromptFeedback.BlockReason)
			}
			lastErr = fmt.Errorf("gemini profile generation blocked due to %s after %d attempts", blockReasonDetail, attempt+1)
		} else {
			lastErr = fmt.Errorf("no valid profile response choices returned from Gemini after %d attempts (finishReason: %s)", attempt+1, finishReason)
		}
		break // Exit loop
	}

	// If loop finished without returning success, return the last recorded error
	return nil, lastErr
}

func logProfileGenerationSuccess(startTime time.Time, apiDuration time.Duration, profiles map[int64]*db.UserProfile, tokenCount int) {
	slog.Info("Gemini profile generation completed",
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"profile_count", len(profiles),
		"tokens", tokenCount)
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

	endIndex := strings.LastIndex(input, "```")
	if endIndex == -1 || endIndex <= startIndex {
		return strings.TrimSpace(input)
	}

	extracted := input[startIndex:endIndex]
	return strings.TrimSpace(extracted)
}

// formatMessage is defined in core.go
