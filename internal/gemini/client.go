// Package gemini implements integration with Google's Gemini AI API.
// It provides natural language processing capabilities for the bot.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
)

// Client defines the interface for AI operations used throughout the application.
// It provides methods for generating replies, analyzing images, and creating user profiles.
type Client interface {
	GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string) (string, error)

	GenerateProfiles(ctx context.Context, messages []*database.Message, existingProfiles map[int64]*database.UserProfile, botID int64, botUsername, botFirstName string) (map[int64]*database.UserProfile, error)

	GenerateImageAnalysis(ctx context.Context, messages []*database.Message, mimeType string, imageData []byte, botID int64, botUsername, botFirstName string) (string, error)
}

type sdkClient struct {
	genaiClient      *genai.Client
	log              *slog.Logger
	contentConfig    *genai.GenerateContentConfig
	defaultModelName string
	maxRetries       int
	retryDelay       time.Duration
}

func (c *sdkClient) prependBotHeader(cfg *genai.GenerateContentConfig, botUsername, botFirstName string) *genai.GenerateContentConfig {
	copyCfg := *cfg
	header := fmt.Sprintf(MentionSystemInstructionHeader, botFirstName, botUsername, botUsername)

	var existingText string
	if cfg.SystemInstruction != nil && len(cfg.SystemInstruction.Parts) > 0 {
		existingText = cfg.SystemInstruction.Parts[0].Text
	}

	copyCfg.SystemInstruction = &genai.Content{
		Parts: []*genai.Part{
			{Text: header + existingText},
		},
	}
	return &copyCfg
}

func formatMessageForAI(m *database.Message) string {
	return fmt.Sprintf("[%s] UID %d: %s", m.Timestamp.Format("2006-01-02 15:04:05"), m.UserID, m.Content)
}

// NewClient creates a new Gemini AI client with the provided configuration.
// It initializes the connection to the Gemini API and sets up necessary parameters.
func NewClient(
	ctx context.Context,
	cfg config.GeminiConfig,
	log *slog.Logger,
) (Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}

	gi, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	baseCfg := &genai.GenerateContentConfig{
		Temperature: &cfg.Temperature,

		SafetySettings: []*genai.SafetySetting{
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
		},
	}

	if cfg.SystemInstruction != "" {
		baseCfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: cfg.SystemInstruction}}}
	}

	logger := log.With("component", "gemini_client")
	logger.Info("Gemini client initialized successfully", "model", cfg.ModelName)
	return &sdkClient{
		genaiClient:      gi,
		log:              logger,
		contentConfig:    baseCfg,
		defaultModelName: cfg.ModelName,
		maxRetries:       cfg.MaxRetries,
		retryDelay:       time.Duration(cfg.RetryDelaySeconds) * time.Second,
	}, nil
}

func (c *sdkClient) generateContentWithRetries(ctx context.Context, modelName string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	var resp *genai.GenerateContentResponse
	var err error

	for i := 0; i <= c.maxRetries; i++ {
		resp, err = c.genaiClient.Models.GenerateContent(ctx, modelName, contents, cfg)
		if err == nil {
			return resp, nil
		}

		c.log.WarnContext(ctx, "Gemini API call failed, checking for retry", "attempt", i+1, "max_retries", c.maxRetries, "error", err)

		var genAiAPIError *genai.APIError
		// Use errors.As to check if the error (or an error it wraps) is a *genai.APIError
		if errors.As(err, &genAiAPIError) && (genAiAPIError.Code == 500 || genAiAPIError.Code == 503) { // Retriable HTTP codes
			if i < c.maxRetries {
				c.log.InfoContext(ctx, "Retrying Gemini API call due to retriable APIError", "delay", c.retryDelay, "code", genAiAPIError.Code)
				time.Sleep(c.retryDelay)
				continue
			}
			// Max retries reached for a retriable genai.APIError
			c.log.ErrorContext(ctx, "Gemini API call failed after max retries with APIError", "error", err, "code", genAiAPIError.Code)
			return nil, fmt.Errorf("gemini API call failed after %d retries (APIError code %d): %w", c.maxRetries, genAiAPIError.Code, err)
		}

		// Not a retriable genai.APIError (either not genai.APIError, or not a 500/503 code, or errors.As returned false)
		c.log.ErrorContext(ctx, "Gemini API call failed with non-retriable error", "error", err)
		return nil, fmt.Errorf("gemini API call failed: %w", err) // Return the original error, wrapped
	}
	// This part of the code should be unreachable if maxRetries >= 0,
	// as the loop's conditions will always lead to a return or continue.
	// Returning the last error to satisfy the compiler and cover edge cases.
	return nil, err
}

func (c *sdkClient) GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string) (string, error) {
	c.log.DebugContext(ctx, "Generating reply", "message_count", len(messages))

	var contents []*genai.Content
	for _, m := range messages {
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}

	copyCfg := *c.contentConfig
	copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{URLContext: &genai.URLContext{}})
	cfgWithHeader := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	resp, err := c.generateContentWithRetries(ctx, c.defaultModelName, contents, cfgWithHeader)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini reply generation failed", "error", err)
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	return c.extractTextFromResponse(ctx, resp)
}

var userProfileSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"user_id":          {Type: genai.TypeString, Description: "The user ID as a string."},
		"aliases":          {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}, Description: "List of known aliases or names mentioned."},
		"origin_location":  {Type: genai.TypeString, Description: "Inferred origin location (city/country). Empty if unknown."},
		"current_location": {Type: genai.TypeString, Description: "Inferred current location (city/country). Empty if unknown."},
		"age_range":        {Type: genai.TypeString, Description: "Inferred age range (e.g., '20-30', 'teenager', 'senior'). Empty if unknown."},
		"traits":           {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}, Description: "List of inferred personality traits, interests, or characteristics."},
	},
	Required: []string{"user_id", "aliases", "origin_location", "current_location", "age_range", "traits"},
}

var profileListSchema = &genai.Schema{
	Type:        genai.TypeArray,
	Description: "A list of updated user profiles based on the provided conversation messages and existing profiles.",
	Items:       userProfileSchema,
}

func (c *sdkClient) GenerateProfiles(
	ctx context.Context,
	messages []*database.Message,
	existingProfiles map[int64]*database.UserProfile,
	botID int64, botUsername, botFirstName string,
) (map[int64]*database.UserProfile, error) {
	c.log.DebugContext(ctx, "Generating profiles using JSON schema mode", "message_count", len(messages), "existing_profile_count", len(existingProfiles))
	if len(messages) == 0 {
		c.log.InfoContext(ctx, "No messages provided for profile generation, returning existing profiles.")
		return existingProfiles, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(ProfileAnalyzerSystemInstruction, botID, botUsername, botFirstName))

	existingJSON, err := json.MarshalIndent(existingProfiles, "", "  ")
	if err != nil {
		c.log.WarnContext(ctx, "Failed to marshal existing profiles for prompt, using empty object.", "error", err)
		sb.WriteString("{}")
	} else {
		sb.Write(existingJSON)
	}
	sb.WriteString("\n\nMessages:\n")

	for _, m := range messages {
		sb.WriteString(formatMessageForAI(m) + "\n")
	}

	contents := []*genai.Content{genai.NewContentFromText(sb.String(), genai.RoleUser)}

	copyCfg := *c.contentConfig
	copyCfg.Tools = nil
	copyCfg.ResponseMIMEType = "application/json"
	copyCfg.ResponseSchema = profileListSchema

	resp, err := c.generateContentWithRetries(ctx, c.defaultModelName, contents, &copyCfg)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini profiles generation API call failed", "error", err)
		return nil, fmt.Errorf("failed to generate profiles: %w", err)
	}

	jsonText, err := c.extractTextFromResponse(ctx, resp)
	if err != nil {
		if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
			c.log.ErrorContext(ctx, "Gemini profiles generation blocked", "reason", resp.PromptFeedback.BlockReason, "message", resp.PromptFeedback.BlockReasonMessage)
			return nil, fmt.Errorf("gemini profiles generation blocked: %s", resp.PromptFeedback.BlockReasonMessage)
		}
		c.log.ErrorContext(ctx, "Failed to extract JSON text from Gemini response", "error", err)
		return nil, fmt.Errorf("failed to extract profiles response: %w", err)
	}

	type ProfileUpdate struct {
		UserID          string   `json:"user_id"`
		Aliases         []string `json:"aliases"`
		OriginLocation  string   `json:"origin_location"`
		CurrentLocation string   `json:"current_location"`
		AgeRange        string   `json:"age_range"`
		Traits          []string `json:"traits"`
	}

	var updates []ProfileUpdate
	if err := json.Unmarshal([]byte(jsonText), &updates); err != nil {
		c.log.ErrorContext(ctx, "Failed to parse profiles JSON array from Gemini response", "error", err, "response_text", jsonText)
		return nil, fmt.Errorf("invalid profiles JSON array received: %w", err)
	}

	result := make(map[int64]*database.UserProfile)

	for id, profile := range existingProfiles {
		result[id] = profile
	}

	parsedCount := 0
	for _, update := range updates {
		userID, err := strconv.ParseInt(update.UserID, 10, 64)
		if err != nil {
			c.log.WarnContext(ctx, "Invalid user ID string in JSON response, skipping entry", "user_id_str", update.UserID, "error", err)
			continue
		}

		result[userID] = &database.UserProfile{
			UserID:          userID,
			Aliases:         strings.Join(update.Aliases, ","),
			OriginLocation:  update.OriginLocation,
			CurrentLocation: update.CurrentLocation,
			AgeRange:        update.AgeRange,
			Traits:          strings.Join(update.Traits, ","),
		}
		parsedCount++
	}

	c.log.DebugContext(ctx, "Successfully parsed user profiles from Gemini JSON response",
		"updates_received", len(updates), "profiles_parsed", parsedCount, "final_profile_count", len(result))
	return result, nil
}

func (c *sdkClient) GenerateImageAnalysis(
	ctx context.Context,
	messages []*database.Message,
	mimeType string,
	imageData []byte,
	botID int64, botUsername, botFirstName string,
) (string, error) {
	c.log.DebugContext(ctx, "Generating image analysis", "image_size", len(imageData), "mime_type", mimeType, "message_count", len(messages))
	if len(imageData) == 0 || mimeType == "" {
		return "", fmt.Errorf("image data and MIME type are required for analysis")
	}

	var contents []*genai.Content
	for _, m := range messages {
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}

	contents = append(contents, genai.NewContentFromParts([]*genai.Part{genai.NewPartFromBytes(imageData, mimeType)}, genai.RoleUser))

	copyCfg := *c.contentConfig
	copyCfg.Tools = nil
	cfgWithHeader := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	resp, err := c.generateContentWithRetries(ctx, c.defaultModelName, contents, cfgWithHeader)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini image analysis API call failed", "error", err)
		return "", fmt.Errorf("gemini image analysis failed: %w", err)
	}

	return c.extractTextFromResponse(ctx, resp)
}

func (c *sdkClient) extractTextFromResponse(ctx context.Context, resp *genai.GenerateContentResponse) (string, error) {
	op := "gemini_operation"
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			parts := strings.Split(fn.Name(), ".")
			if len(parts) >= 2 {
				op = parts[len(parts)-1]
			}
		}
	}

	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
		reasonMsg := fmt.Sprintf("%v", resp.PromptFeedback.BlockReason)
		if resp.PromptFeedback.BlockReasonMessage != "" {
			reasonMsg = resp.PromptFeedback.BlockReasonMessage
		}
		c.log.ErrorContext(ctx, "Gemini request blocked", "operation", op, "reason", reasonMsg)
		return "", fmt.Errorf("%s blocked by safety filter: %s", op, reasonMsg)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		finishReason := "unknown"
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonUnspecified {
			finishReason = fmt.Sprintf("%v", resp.Candidates[0].FinishReason)
		}
		c.log.WarnContext(ctx, "Gemini response missing candidates or content", "operation", op, "finish_reason", finishReason)

		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			return "", fmt.Errorf("%s returned no content, finish reason: %s", op, finishReason)
		}

		return "", fmt.Errorf("%s returned empty content", op)
	}

	rawText := resp.Text()

	re := regexp.MustCompile(`(?m)^(?:\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] UID \d+: )+`)
	cleanText := re.ReplaceAllString(rawText, "")

	if cleanText == "" {
		c.log.WarnContext(ctx, "Gemini response text is empty after stripping prefixes", "operation", op, "raw_text", rawText)

		return "", fmt.Errorf("%s returned empty text after processing", op)
	}

	return cleanText, nil
}
