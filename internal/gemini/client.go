// Package gemini provides an interface and implementation for interacting with the Gemini AI API.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"google.golang.org/genai"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
)

// Client defines the interface for interacting with the Gemini AI model.
type Client interface {
	GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error)
	GenerateProfiles(ctx context.Context, messages []*database.Message, existingProfiles map[int64]*database.UserProfile) (map[int64]*database.UserProfile, error)
	GenerateImageAnalysis(ctx context.Context, messages []*database.Message, mimeType string, imageData []byte, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error)
}

// sdkClient implements Client using Google GenAI.
type sdkClient struct {
	genaiClient      *genai.Client
	log              *slog.Logger
	contentConfig    *genai.GenerateContentConfig
	defaultModelName string
}

// prependBotHeader injects the dynamic system instruction header.
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

// formatMessageForAI formats a message for AI consumption.
func formatMessageForAI(m *database.Message) string {
	// Format: [YYYY-MM-DD HH:MM:SS] UID 12345: message content
	return fmt.Sprintf("[%s] UID %d: %s", m.Timestamp.Format("2006-01-02 15:04:05"), m.UserID, m.Content)
}

// NewClient constructs a Gemini client without bot identity.
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

	// base generation config
	baseCfg := &genai.GenerateContentConfig{
		Temperature: &cfg.Temperature,
		SafetySettings: []*genai.SafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
		},
	}
	if cfg.SystemInstruction != "" {
		baseCfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: cfg.SystemInstruction}}}
	}

	logger := log.With("component", "gemini_client")
	logger.Info("Gemini client initialized successfully")
	return &sdkClient{
		genaiClient:      gi,
		log:              logger,
		contentConfig:    baseCfg,
		defaultModelName: cfg.ModelName,
	}, nil
}

// GenerateReply sends conversation to Gemini with a dynamic header.
func (c *sdkClient) GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error) {
	c.log.DebugContext(ctx, "Generating reply", "count", len(messages))
	var contents []*genai.Content
	for _, m := range messages {
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}

	copyCfg := *c.contentConfig
	if searchGrounding {
		copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}
	cfgWith := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, cfgWith)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini reply failed", "error", err)
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}
	return c.extractTextFromResponse(ctx, resp)
}

// Define the expected JSON schema for a single user profile update
var userProfileSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"user_id":          {Type: genai.TypeString, Description: "The user ID as a string."},
		"aliases":          {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}, Description: "List of known aliases."},
		"origin_location":  {Type: genai.TypeString, Description: "Inferred origin location."},
		"current_location": {Type: genai.TypeString, Description: "Inferred current location."},
		"age_range":        {Type: genai.TypeString, Description: "Inferred age range (e.g., 20-30)."},
		"traits":           {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}, Description: "List of inferred personality traits or characteristics."},
	},
	Required: []string{"user_id", "aliases", "origin_location", "current_location", "age_range", "traits"},
}

// Define the top-level schema: an array of user profile updates
var profileListSchema = &genai.Schema{
	Type:        genai.TypeArray,
	Description: "A list of updated user profiles based on the conversation.",
	Items:       userProfileSchema,
}

// GenerateProfiles analyzes messages and returns updated profiles using JSON mode with a defined schema.
func (c *sdkClient) GenerateProfiles(
	ctx context.Context,
	messages []*database.Message,
	existingProfiles map[int64]*database.UserProfile,
) (map[int64]*database.UserProfile, error) {
	c.log.DebugContext(ctx, "Generating profiles using JSON schema mode", "count", len(messages))
	if len(messages) == 0 {
		return existingProfiles, nil // Return early if no messages to process
	}

	var sb strings.Builder
	// Simplified prompt focusing on the task, letting the schema enforce structure.
	sb.WriteString(ProfileAnalyzerSystemInstruction)
	existingJSON, err := json.MarshalIndent(existingProfiles, "", "  ")
	if err != nil {
		c.log.WarnContext(ctx, "Failed to marshal existing profiles for prompt", "error", err)
		sb.WriteString("{}") // Add empty JSON object placeholder
	} else {
		sb.Write(existingJSON)
	}

	sb.WriteString("\n\nMessages:\n")
	for _, m := range messages {
		sb.WriteString(formatMessageForAI(m) + "\n")
	}

	contents := []*genai.Content{genai.NewContentFromText(sb.String(), genai.RoleUser)}

	// Configure for JSON output using the defined schema
	copyCfg := *c.contentConfig
	copyCfg.Tools = nil                           // Disable tools
	copyCfg.ResponseMIMEType = "application/json" // Ensure JSON output
	copyCfg.ResponseSchema = profileListSchema    // Enforce the specific schema

	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, &copyCfg)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini profiles generation failed", "error", err)
		return nil, fmt.Errorf("failed to generate profiles: %w", err)
	}

	jsonText, err := c.extractTextFromResponse(ctx, resp)
	if err != nil {
		// Check for blocking specifically
		if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
			c.log.ErrorContext(ctx, "Gemini profiles generation blocked", "reason", resp.PromptFeedback.BlockReason, "message", resp.PromptFeedback.BlockReasonMessage)
			return nil, fmt.Errorf("gemini profiles generation blocked: %s", resp.PromptFeedback.BlockReasonMessage)
		}
		c.log.ErrorContext(ctx, "Failed to extract text from Gemini response", "error", err)
		return nil, fmt.Errorf("failed to extract profiles response: %w", err)
	}

	// Define a temporary struct to match the schema for unmarshalling
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
		c.log.ErrorContext(ctx, "Failed to parse profiles JSON array from Gemini response",
			"error", err,
			"response_text", jsonText) // Log the raw text received
		return nil, fmt.Errorf("invalid profiles JSON array received: %w", err)
	}

	// Convert the array of updates back into the map format
	result := make(map[int64]*database.UserProfile)
	// Start with existing profiles, potentially overwriting with updates
	for id, profile := range existingProfiles {
		result[id] = profile
	}

	parsedCount := 0
	for _, update := range updates {
		userID, err := strconv.ParseInt(update.UserID, 10, 64)
		if err != nil {
			c.log.WarnContext(ctx, "Invalid user ID string in JSON response, skipping entry", "user_id_str", update.UserID, "error", err)
			continue // Skip this entry
		}

		// Update or create the profile in the result map
		// Join string slices into comma-separated strings for the DB model
		result[userID] = &database.UserProfile{
			UserID:          userID,                            // Ensure UserID field is set correctly
			Aliases:         strings.Join(update.Aliases, ","), // Join slice
			OriginLocation:  update.OriginLocation,
			CurrentLocation: update.CurrentLocation,
			AgeRange:        update.AgeRange,
			Traits:          strings.Join(update.Traits, ","), // Join slice
			// Removed LastUpdated: time.Now(), as it's not in the DB model
		}
		parsedCount++
	}

	c.log.DebugContext(ctx, "Successfully parsed user profiles from Gemini JSON schema response",
		"updates_received", len(updates),
		"profiles_parsed", parsedCount,
		"final_profile_count", len(result))
	return result, nil
}

// GenerateImageAnalysis sends image + context to Gemini.
func (c *sdkClient) GenerateImageAnalysis(
	ctx context.Context,
	messages []*database.Message,
	mimeType string,
	imageData []byte,
	botID int64, botUsername, botFirstName string,
	searchGrounding bool,
) (string, error) {
	c.log.DebugContext(ctx, "Generating image analysis", "size", len(imageData), "mime", mimeType)
	if len(imageData) == 0 || mimeType == "" {
		return "", fmt.Errorf("image data and MIME type required")
	}

	var contents []*genai.Content
	for _, m := range messages {
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}
	// Add the image part last
	contents = append(contents, genai.NewContentFromParts([]*genai.Part{genai.NewPartFromBytes(imageData, mimeType)}, genai.RoleUser))

	copyCfg := *c.contentConfig
	if searchGrounding {
		copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}
	cfgWith := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, cfgWith)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini image analysis failed", "error", err)
		return "", fmt.Errorf("gemini image analysis: %w", err)
	}
	return c.extractTextFromResponse(ctx, resp)
}

// extractTextFromResponse handles blocking and extracts text.
func (c *sdkClient) extractTextFromResponse(ctx context.Context, resp *genai.GenerateContentResponse) (string, error) {
	op := "unknown"
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			parts := strings.Split(fn.Name(), ".")
			if len(parts) >= 2 {
				op = parts[len(parts)-1]
			}
		}
	}
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
		// Use fmt.Sprintf for reason codes instead of .String()
		reasonMsg := fmt.Sprintf("%v", resp.PromptFeedback.BlockReason)
		if resp.PromptFeedback.BlockReasonMessage != "" {
			reasonMsg = resp.PromptFeedback.BlockReasonMessage
		}
		c.log.ErrorContext(ctx, "Gemini request blocked", "operation", op, "reason", reasonMsg)
		return "", fmt.Errorf("%s blocked: %s", op, reasonMsg)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		finishReason := "unknown"
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonUnspecified {
			// Use fmt.Sprintf for reason codes instead of .String()
			finishReason = fmt.Sprintf("%v", resp.Candidates[0].FinishReason)
		}
		c.log.WarnContext(ctx, "Gemini response missing content", "operation", op, "finish_reason", finishReason)
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			return "", fmt.Errorf("%s returned no candidates or content, finish reason: %s", op, finishReason)
		}
		return "", fmt.Errorf("%s returned no candidates or content", op)
	}

	// Use resp.Text() to retrieve full response text instead of manual part iteration
	rawText := resp.Text()
	// Strip timestamp/UID prefixes from response
	re := regexp.MustCompile(`(?m)^(?:\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] UID \d+: )+`)
	cleanText := re.ReplaceAllString(rawText, "")
	if cleanText == "" {
		c.log.WarnContext(ctx, "Gemini response text is empty after processing parts", "operation", op)
		return "", fmt.Errorf("%s returned empty text", op)
	}
	return cleanText, nil
}
