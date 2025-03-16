// Package ai provides artificial intelligence capabilities for MurailoBot,
// handling message response generation and user profile analysis using
// OpenAI's API or compatible services.
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
	"github.com/sashabaranov/go-openai"
)

// Client implements AI functionality for generating responses to user messages
// and analyzing user profiles based on message history. It uses OpenAI's API
// or compatible services to provide natural language processing capabilities.
type Client struct {
	client             *openai.Client
	model              string
	temperature        float32
	instruction        string
	profileInstruction string
	timeout            time.Duration
	maxContextTokens   int
	storage            *db.DB
	botInfo            BotInfo
}

// New creates a new AI client with the provided configuration and storage.
// It initializes the OpenAI client with the appropriate API token and base URL.
func New(cfg *config.Config, storage *db.DB) (*Client, error) {
	if storage == nil {
		return nil, errors.New("nil storage")
	}

	aiConfig := openai.DefaultConfig(cfg.AIToken)
	aiConfig.BaseURL = cfg.AIBaseURL

	client := &Client{
		client:             openai.NewClientWithConfig(aiConfig),
		model:              cfg.AIModel,
		temperature:        cfg.AITemperature,
		instruction:        cfg.AIInstruction,
		profileInstruction: cfg.AIProfileInstruction,
		timeout:            cfg.AITimeout,
		maxContextTokens:   cfg.AIMaxContextTokens,
		storage:            storage,
	}

	return client, nil
}

// SetBotInfo configures the bot's identity information used for message processing
// and profile generation. This information helps the AI distinguish between bot
// and user messages, and properly handle the bot's own profile.
//
// Returns an error if the provided bot information is invalid.
func (c *Client) SetBotInfo(info BotInfo) error {
	if info.UserID <= 0 {
		return errors.New("invalid bot user ID")
	}

	if info.Username == "" {
		return errors.New("empty bot username")
	}

	c.botInfo = info

	return nil
}

func formatMessage(msg *db.Message) string {
	return fmt.Sprintf("[%s] UID %d: %s",
		msg.Timestamp.Format(time.RFC3339),
		msg.UserID,
		msg.Content)
}

func (c *Client) createSystemPrompt(userProfiles map[int64]*db.UserProfile) string {
	// Use the bot's display name if available, otherwise fall back to username
	displayName := c.botInfo.Username
	if c.botInfo.DisplayName != "" {
		displayName = c.botInfo.DisplayName
	}

	// Create a personalized header that defines the bot's identity and expected behavior
	// This helps the AI model understand its role in the conversation
	botIdentityHeader := fmt.Sprintf(
		"You are %s, a Telegram bot in a group chat. When someone mentions you with @%s, "+
			"your task is to respond to their message. Messages will include the @%s mention - "+
			"this is normal and expected. Always respond directly to the content of the message. "+
			"Even if the message doesn't contain a clear question, assume it's directed at you "+
			"and respond appropriately.\n\n",
		displayName, c.botInfo.Username, c.botInfo.Username)

	// Combine the identity header with the configured instruction text
	systemPrompt := botIdentityHeader + c.instruction

	// If user profiles are available, append them to provide context about the participants
	if len(userProfiles) > 0 {
		var profileInfo strings.Builder

		// Add a section header and format explanation for the profiles
		profileInfo.WriteString("\n\n## USER PROFILES\n")
		profileInfo.WriteString("Format: UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]\n\n")

		// Sort user IDs for consistent ordering in the prompt
		userIDs := make([]int64, 0, len(userProfiles))
		for id := range userProfiles {
			userIDs = append(userIDs, id)
		}

		sort.Slice(userIDs, func(i, j int) bool {
			return userIDs[i] < userIDs[j]
		})

		// Process each user profile, with special handling for the bot's own profile
		for _, id := range userIDs {
			if id == c.botInfo.UserID {
				// For the bot's own profile, use a standardized format
				botDisplayNames := c.botInfo.Username
				if c.botInfo.DisplayName != "" && c.botInfo.DisplayName != c.botInfo.Username {
					botDisplayNames = fmt.Sprintf("%s, %s", c.botInfo.DisplayName, c.botInfo.Username)
				}

				profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | Internet | Internet | N/A | Group Chat Bot\n", id, botDisplayNames))

				continue
			}

			// For regular users, use their stored profile information
			profile := userProfiles[id]
			profileInfo.WriteString(profile.FormatPipeDelimited() + "\n")
		}

		systemPrompt += profileInfo.String()
	}

	return systemPrompt
}

// GenerateResponse creates an AI-generated response to a user message.
// It uses the provided context, user profiles, and conversation history
// to generate a contextually appropriate response.
//
// The method handles token budget management to ensure the context fits
// within the model's limitations, and properly formats messages for the AI.
//
// Returns the generated response as a string, or an error if the generation fails.
func (c *Client) GenerateResponse(ctx context.Context, request *Request) (string, error) {
	if request == nil {
		return "", errors.New("nil request")
	}

	if request.UserID <= 0 {
		return "", errors.New("invalid user ID")
	}

	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return "", errors.New("empty user message")
	}

	// Create a system prompt that includes bot identity and user profiles
	// This provides context for the AI to generate appropriate responses
	systemPrompt := c.createSystemPrompt(request.UserProfiles)

	// Initialize the messages array with the system prompt
	messages := []openai.ChatCompletionMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	// Calculate token usage for system prompt and current message
	// This is needed for token budget management
	systemPromptTokens := utils.EstimateTokens(systemPrompt)
	currentMsgTokens := utils.EstimateTokens(request.Message)

	// Select a subset of recent messages that fit within the token budget
	// This ensures we don't exceed the model's context window limits
	selectedMessages := utils.SelectMessages(
		c.maxContextTokens,
		request.RecentMessages,
		systemPromptTokens,
		currentMsgTokens,
	)

	// Add the selected conversation history to the messages array
	// Properly identifying bot messages as "assistant" and user messages as "user"
	for _, msg := range selectedMessages {
		role := "user"
		if msg.UserID == c.botInfo.UserID {
			role = "assistant"
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: formatMessage(msg),
		})
	}

	// Format and add the current user message
	currentMsg := formatMessage(&db.Message{
		UserID:    request.UserID,
		Content:   request.Message,
		Timestamp: time.Now().UTC(),
	})

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	// Create a timeout context to prevent hanging on API calls
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Make the API call to generate a response
	resp, err := c.client.CreateChatCompletion(timeoutCtx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
	})
	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no response choices returned")
	}

	// Clean and normalize the response text before returning it
	result, err := utils.Sanitize(resp.Choices[0].Message.Content)
	if err != nil {
		return "", fmt.Errorf("failed to sanitize response: %w", err)
	}

	return result, nil
}

// GenerateProfiles analyzes user messages to create or update user profiles.
// It processes the provided messages, groups them by user, and uses AI to
// extract information about each user's characteristics, locations, and traits.
//
// The method preserves existing profile data when updating profiles and handles
// special cases like the bot's own profile.
//
// Returns a map of user IDs to updated user profiles, or an error if the
// profile generation fails.
func (c *Client) GenerateProfiles(ctx context.Context, messages []*db.Message, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	if len(messages) == 0 {
		return nil, errors.New("no messages to analyze")
	}

	userMessages := make(map[int64][]*db.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	instruction := getProfileInstruction(c.profileInstruction, c.botInfo)

	chatMessages := []openai.ChatCompletionMessage{
		{
			Role:    "system",
			Content: instruction,
		},
	}

	var msgBuilder strings.Builder

	if len(existingProfiles) > 0 {
		msgBuilder.WriteString("## EXISTING USER PROFILES\n\n")
		msgBuilder.WriteString("{\n  \"users\": {\n")

		i := 0
		for userID, profile := range existingProfiles {
			if i > 0 {
				msgBuilder.WriteString(",\n")
			}

			msgBuilder.WriteString(fmt.Sprintf("    \"%d\": {\n", userID))
			msgBuilder.WriteString(fmt.Sprintf("      \"display_names\": \"%s\",\n", profile.DisplayNames))
			msgBuilder.WriteString(fmt.Sprintf("      \"origin_location\": \"%s\",\n", profile.OriginLocation))
			msgBuilder.WriteString(fmt.Sprintf("      \"current_location\": \"%s\",\n", profile.CurrentLocation))
			msgBuilder.WriteString(fmt.Sprintf("      \"age_range\": \"%s\",\n", profile.AgeRange))
			msgBuilder.WriteString(fmt.Sprintf("      \"traits\": \"%s\"\n", profile.Traits))
			msgBuilder.WriteString("    }")

			i++
		}

		msgBuilder.WriteString("\n  }\n}\n\n")
	}

	msgBuilder.WriteString("## NEW GROUP CHAT MESSAGES\n\n")

	totalMessages := 0

	for userID, userMsgs := range userMessages {
		msgBuilder.WriteString(fmt.Sprintf("Messages from User %d:\n", userID))

		for _, msg := range userMsgs {
			msgBuilder.WriteString(fmt.Sprintf("[%s] %s\n",
				msg.Timestamp.Format(time.RFC3339),
				msg.Content))

			totalMessages++
		}

		msgBuilder.WriteString("\n")
	}

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: msgBuilder.String(),
	})

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(timeoutCtx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    chatMessages,
		Temperature: c.temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("profile generation failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no response choices returned")
	}

	return parseProfileResponse(resp.Choices[0].Message.Content, userMessages, existingProfiles, c.botInfo)
}

func parseProfileResponse(response string, userMessages map[int64][]*db.Message, existingProfiles map[int64]*db.UserProfile, botInfo BotInfo) (map[int64]*db.UserProfile, error) {
	response = strings.TrimSpace(response)
	if response == "" {
		return nil, errors.New("empty profile response")
	}

	// Extract the JSON content from the response
	// AI models sometimes include explanatory text before/after the JSON
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, errors.New("invalid JSON format in response")
	}

	jsonContent := response[jsonStart : jsonEnd+1]

	// Define a struct that matches the expected JSON structure
	var profileResponse struct {
		Users map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		} `json:"users"`
	}

	// Try to parse the JSON
	err := json.Unmarshal([]byte(jsonContent), &profileResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profile response: %w", err)
	}

	// Initialize an empty users map if none was provided in the response
	if profileResponse.Users == nil {
		profileResponse.Users = make(map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		})
	}

	updatedProfiles := make(map[int64]*db.UserProfile)

	// Process each user profile from the response
	for userIDStr, profile := range profileResponse.Users {
		// Convert string user ID to int64
		var userID int64
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID == 0 {
			slog.Warn("invalid user ID in profile response", "user_id", userIDStr)
			continue
		}

		// Special handling for the bot's own profile
		// This ensures the bot always has a consistent profile
		if userID == botInfo.UserID {
			botDisplayNames := botInfo.Username
			if botInfo.DisplayName != "" && botInfo.DisplayName != botInfo.Username {
				botDisplayNames = fmt.Sprintf("%s, %s", botInfo.DisplayName, botInfo.Username)
			}

			updatedProfiles[userID] = &db.UserProfile{
				UserID:          userID,
				DisplayNames:    botDisplayNames,
				OriginLocation:  "Internet",
				CurrentLocation: "Internet",
				AgeRange:        "N/A",
				Traits:          "Group Chat Bot",
				LastUpdated:     time.Now().UTC(),
			}

			continue
		}

		// Skip users that don't have messages in this batch and don't have existing profiles
		// This prevents creating profiles for users that weren't part of the analysis
		if _, hasMessages := userMessages[userID]; !hasMessages {
			if _, hasProfile := existingProfiles[userID]; !hasProfile {
				continue
			}
		}

		// Create updated profile for the user
		updatedProfiles[userID] = &db.UserProfile{
			UserID:          userID,
			DisplayNames:    profile.DisplayNames,
			OriginLocation:  profile.OriginLocation,
			CurrentLocation: profile.CurrentLocation,
			AgeRange:        profile.AgeRange,
			Traits:          profile.Traits,
			LastUpdated:     time.Now().UTC(),
		}
	}

	return updatedProfiles, nil
}

func getProfileInstruction(configInstruction string, botInfo BotInfo) string {
	botIdentificationAndFixedPart := fmt.Sprintf(`
## BOT IDENTIFICATION [IMPORTANT]
Bot UID: %d
Bot Username: %s
Bot Display Name: %s

### BOT INFLUENCE AWARENESS [IMPORTANT]
- DO NOT attribute traits based on topics introduced by the bot
- If the bot mentions a topic and the user merely responds, this is not evidence of a personal trait
- Only identify traits from topics and interests the user has independently demonstrated
- Ignore creative embellishments that might have been added by the bot in previous responses

## OUTPUT FORMAT [CRITICAL]
Return ONLY a JSON object, no additional text, with this structure:
{
  "users": {
    "[user_id]": {
      "display_names": "Comma-separated list of names/nicknames",
      "origin_location": "Where the user is from",
      "current_location": "Where the user currently lives",
      "age_range": "Approximate age range (20s, 30s, etc.)",
      "traits": "Comma-separated list of personality traits and characteristics"
    }
  }
}`, botInfo.UserID, botInfo.Username, botInfo.DisplayName)

	return fmt.Sprintf("%s\n\n%s", configInstruction, botIdentificationAndFixedPart)
}
