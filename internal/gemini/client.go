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
	// GenerateReply generates a text response based on the provided message history.
	// It includes bot identity information and optionally enables search grounding.
	GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error)

	// GenerateProfiles analyzes message history to create or update user profiles based on a predefined JSON schema.
	GenerateProfiles(ctx context.Context, messages []*database.Message, existingProfiles map[int64]*database.UserProfile) (map[int64]*database.UserProfile, error)

	// GenerateImageAnalysis generates a text response based on message history and an accompanying image.
	// It includes bot identity information and optionally enables search grounding.
	GenerateImageAnalysis(ctx context.Context, messages []*database.Message, mimeType string, imageData []byte, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error)
}

// sdkClient implements the Client interface using the official Google GenAI SDK.
type sdkClient struct {
	genaiClient      *genai.Client // Underlying Google GenAI SDK client.
	log              *slog.Logger
	contentConfig    *genai.GenerateContentConfig // Base configuration for content generation (temperature, safety, system instruction).
	defaultModelName string                       // Default model name (e.g., "gemini-1.5-flash").
}

// prependBotHeader creates a copy of the generation config and prepends
// dynamic bot identity information to the system instruction.
func (c *sdkClient) prependBotHeader(cfg *genai.GenerateContentConfig, botUsername, botFirstName string) *genai.GenerateContentConfig {
	copyCfg := *cfg // Make a shallow copy to avoid modifying the base config.
	header := fmt.Sprintf(MentionSystemInstructionHeader, botFirstName, botUsername, botUsername)

	var existingText string
	if cfg.SystemInstruction != nil && len(cfg.SystemInstruction.Parts) > 0 {
		// Assume the base system instruction is the first part.
		existingText = cfg.SystemInstruction.Parts[0].Text
	}

	// Combine the dynamic header with the existing base instruction.
	copyCfg.SystemInstruction = &genai.Content{
		Parts: []*genai.Part{
			{Text: header + existingText},
		},
	}
	return &copyCfg
}

// formatMessageForAI formats a database message into a string suitable for the AI prompt,
// including timestamp and user ID.
func formatMessageForAI(m *database.Message) string {
	// Format: [YYYY-MM-DD HH:MM:SS] UID 12345: message content
	return fmt.Sprintf("[%s] UID %d: %s", m.Timestamp.Format("2006-01-02 15:04:05"), m.UserID, m.Content)
}

// NewClient creates and initializes a new Gemini client using the provided configuration.
func NewClient(
	ctx context.Context,
	cfg config.GeminiConfig,
	log *slog.Logger,
) (Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}

	// Initialize the underlying Google GenAI SDK client.
	gi, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI, // Use the standard Gemini API backend.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	// Define the base generation configuration.
	baseCfg := &genai.GenerateContentConfig{
		Temperature: &cfg.Temperature,
		// Configure safety settings to allow all content (BLOCK_NONE).
		// Adjust thresholds based on application requirements if needed.
		SafetySettings: []*genai.SafetySetting{
			// Corrected constant names
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
		},
	}
	// Set the base system instruction if provided in the config.
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
	}, nil
}

// GenerateReply sends the conversation history to Gemini for a text-based reply.
// It formats messages, prepends bot identity, and optionally enables search grounding.
func (c *sdkClient) GenerateReply(ctx context.Context, messages []*database.Message, botID int64, botUsername, botFirstName string, searchGrounding bool) (string, error) {
	c.log.DebugContext(ctx, "Generating reply", "message_count", len(messages), "search_grounding", searchGrounding)

	// Convert database messages to Gemini Content objects, assigning roles.
	var contents []*genai.Content
	for _, m := range messages {
		// Corrected type: Use genai.Role directly
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}

	// Create a copy of the base config and potentially add tools.
	copyCfg := *c.contentConfig
	if searchGrounding {
		// Enable Google Search as a tool for grounding the response.
		copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}
	// Prepend the dynamic bot header to the system instruction.
	cfgWithHeader := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	// Call the Gemini API.
	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, cfgWithHeader)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini reply generation failed", "error", err)
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	// Extract and clean the text response.
	return c.extractTextFromResponse(ctx, resp)
}

// userProfileSchema defines the expected JSON structure for a single user profile update.
// Used by Gemini's JSON mode to enforce the output format.
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

// profileListSchema defines the top-level schema: an array of user profile updates.
// This is the schema passed to the Gemini API for the GenerateProfiles function.
var profileListSchema = &genai.Schema{
	Type:        genai.TypeArray,
	Description: "A list of updated user profiles based on the provided conversation messages and existing profiles.",
	Items:       userProfileSchema, // Each item in the array must conform to userProfileSchema.
}

// GenerateProfiles analyzes messages and existing profiles to generate updated profiles using Gemini's JSON mode.
func (c *sdkClient) GenerateProfiles(
	ctx context.Context,
	messages []*database.Message,
	existingProfiles map[int64]*database.UserProfile,
) (map[int64]*database.UserProfile, error) {
	c.log.DebugContext(ctx, "Generating profiles using JSON schema mode", "message_count", len(messages), "existing_profile_count", len(existingProfiles))
	if len(messages) == 0 {
		c.log.InfoContext(ctx, "No messages provided for profile generation, returning existing profiles.")
		return existingProfiles, nil // Return early if no messages to process
	}

	// Construct the prompt for the AI.
	var sb strings.Builder
	sb.WriteString(ProfileAnalyzerSystemInstruction) // Base instruction for the profile analysis task.
	// Include existing profiles in the prompt as context.
	existingJSON, err := json.MarshalIndent(existingProfiles, "", "  ")
	if err != nil {
		c.log.WarnContext(ctx, "Failed to marshal existing profiles for prompt, using empty object.", "error", err)
		sb.WriteString("{}") // Use empty JSON object as placeholder if marshalling fails.
	} else {
		sb.Write(existingJSON)
	}
	sb.WriteString("\n\nMessages:\n")
	// Append formatted messages to the prompt.
	for _, m := range messages {
		sb.WriteString(formatMessageForAI(m) + "\n")
	}

	// Prepare the content for the API call (single user message containing the full prompt).
	contents := []*genai.Content{genai.NewContentFromText(sb.String(), genai.RoleUser)}

	// Configure the API call for JSON output using the defined schema.
	copyCfg := *c.contentConfig
	copyCfg.Tools = nil                           // Disable tools (like search) for profile generation.
	copyCfg.ResponseMIMEType = "application/json" // Request JSON output.
	copyCfg.ResponseSchema = profileListSchema    // Enforce the specific schema for the response.

	// Call the Gemini API.
	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, &copyCfg)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini profiles generation API call failed", "error", err)
		return nil, fmt.Errorf("failed to generate profiles: %w", err)
	}

	// Extract the JSON text from the response.
	jsonText, err := c.extractTextFromResponse(ctx, resp)
	if err != nil {
		// Check specifically if the request was blocked by safety filters.
		if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
			c.log.ErrorContext(ctx, "Gemini profiles generation blocked", "reason", resp.PromptFeedback.BlockReason, "message", resp.PromptFeedback.BlockReasonMessage)
			return nil, fmt.Errorf("gemini profiles generation blocked: %s", resp.PromptFeedback.BlockReasonMessage)
		}
		c.log.ErrorContext(ctx, "Failed to extract JSON text from Gemini response", "error", err)
		return nil, fmt.Errorf("failed to extract profiles response: %w", err)
	}

	// Define a temporary struct matching the JSON schema for unmarshalling.
	// This bridges the gap between the JSON structure and the database model.
	type ProfileUpdate struct {
		UserID          string   `json:"user_id"`
		Aliases         []string `json:"aliases"`
		OriginLocation  string   `json:"origin_location"`
		CurrentLocation string   `json:"current_location"`
		AgeRange        string   `json:"age_range"`
		Traits          []string `json:"traits"`
	}

	// Unmarshal the JSON array response into the temporary struct slice.
	var updates []ProfileUpdate
	if err := json.Unmarshal([]byte(jsonText), &updates); err != nil {
		c.log.ErrorContext(ctx, "Failed to parse profiles JSON array from Gemini response", "error", err, "response_text", jsonText)
		return nil, fmt.Errorf("invalid profiles JSON array received: %w", err)
	}

	// Convert the array of updates back into the map format required by the Store.
	result := make(map[int64]*database.UserProfile)
	// Start with existing profiles, allowing updates to overwrite them.
	for id, profile := range existingProfiles {
		result[id] = profile
	}

	parsedCount := 0
	for _, update := range updates {
		// Convert UserID string from JSON back to int64.
		userID, err := strconv.ParseInt(update.UserID, 10, 64)
		if err != nil {
			c.log.WarnContext(ctx, "Invalid user ID string in JSON response, skipping entry", "user_id_str", update.UserID, "error", err)
			continue // Skip this profile update if UserID is invalid.
		}

		// Update or create the profile in the result map.
		// Convert string slices (aliases, traits) from JSON to comma-separated strings for the database model.
		result[userID] = &database.UserProfile{
			UserID:          userID,
			Aliases:         strings.Join(update.Aliases, ","),
			OriginLocation:  update.OriginLocation,
			CurrentLocation: update.CurrentLocation,
			AgeRange:        update.AgeRange,
			Traits:          strings.Join(update.Traits, ","),
			// CreatedAt/UpdatedAt are handled by the Store during saving.
		}
		parsedCount++
	}

	c.log.DebugContext(ctx, "Successfully parsed user profiles from Gemini JSON response",
		"updates_received", len(updates), "profiles_parsed", parsedCount, "final_profile_count", len(result))
	return result, nil
}

// GenerateImageAnalysis sends message history and image data to Gemini for analysis.
func (c *sdkClient) GenerateImageAnalysis(
	ctx context.Context,
	messages []*database.Message,
	mimeType string,
	imageData []byte,
	botID int64, botUsername, botFirstName string,
	searchGrounding bool,
) (string, error) {
	c.log.DebugContext(ctx, "Generating image analysis", "image_size", len(imageData), "mime_type", mimeType, "message_count", len(messages), "search_grounding", searchGrounding)
	if len(imageData) == 0 || mimeType == "" {
		return "", fmt.Errorf("image data and MIME type are required for analysis")
	}

	// Convert database messages to Gemini Content objects.
	var contents []*genai.Content
	for _, m := range messages {
		// Corrected type: Use genai.Role directly
		var role genai.Role = genai.RoleUser
		if m.UserID == botID {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(formatMessageForAI(m), role))
	}
	// Append the image data as the last part of the user message.
	contents = append(contents, genai.NewContentFromParts([]*genai.Part{genai.NewPartFromBytes(imageData, mimeType)}, genai.RoleUser))

	// Prepare configuration, optionally adding search grounding.
	copyCfg := *c.contentConfig
	if searchGrounding {
		copyCfg.Tools = append(copyCfg.Tools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}
	cfgWithHeader := c.prependBotHeader(&copyCfg, botUsername, botFirstName)

	// Call the Gemini API.
	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.defaultModelName, contents, cfgWithHeader)
	if err != nil {
		c.log.ErrorContext(ctx, "Gemini image analysis API call failed", "error", err)
		return "", fmt.Errorf("gemini image analysis failed: %w", err)
	}

	// Extract and clean the text response.
	return c.extractTextFromResponse(ctx, resp)
}

// extractTextFromResponse processes the Gemini API response, handling potential blocking
// or empty content, and extracts the generated text, stripping unwanted prefixes.
func (c *sdkClient) extractTextFromResponse(ctx context.Context, resp *genai.GenerateContentResponse) (string, error) {
	// Determine the calling function name for logging context.
	op := "gemini_operation" // Default operation name
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			parts := strings.Split(fn.Name(), ".")
			if len(parts) >= 2 {
				op = parts[len(parts)-1] // Get the function name (e.g., GenerateReply)
			}
		}
	}

	// Check if the prompt or response was blocked by safety filters.
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
		reasonMsg := fmt.Sprintf("%v", resp.PromptFeedback.BlockReason) // Use enum value directly
		if resp.PromptFeedback.BlockReasonMessage != "" {
			reasonMsg = resp.PromptFeedback.BlockReasonMessage // Use specific message if available
		}
		c.log.ErrorContext(ctx, "Gemini request blocked", "operation", op, "reason", reasonMsg)
		return "", fmt.Errorf("%s blocked by safety filter: %s", op, reasonMsg)
	}

	// Check if the response contains any candidates and content.
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		finishReason := "unknown"
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonUnspecified {
			finishReason = fmt.Sprintf("%v", resp.Candidates[0].FinishReason) // Use enum value
		}
		c.log.WarnContext(ctx, "Gemini response missing candidates or content", "operation", op, "finish_reason", finishReason)
		// Return error if finish reason indicates a problem (not STOP).
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			return "", fmt.Errorf("%s returned no content, finish reason: %s", op, finishReason)
		}
		// If finish reason is STOP but content is empty, return a generic error.
		return "", fmt.Errorf("%s returned empty content", op)
	}

	// Use resp.Text() helper to concatenate text parts correctly.
	rawText := resp.Text()

	// Strip potential timestamp/UID prefixes that the model might add in its response.
	// This regex matches lines starting with "[YYYY-MM-DD HH:MM:SS] UID <number>: ".
	re := regexp.MustCompile(`(?m)^(?:\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] UID \d+: )+`)
	cleanText := re.ReplaceAllString(rawText, "")

	if cleanText == "" {
		c.log.WarnContext(ctx, "Gemini response text is empty after stripping prefixes", "operation", op, "raw_text", rawText)
		// Consider if this should be an error or if an empty string is a valid (though unusual) response.
		// Returning an error for now.
		return "", fmt.Errorf("%s returned empty text after processing", op)
	}

	return cleanText, nil
}
