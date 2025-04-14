// Package openai provides an OpenAI-based implementation of the AI service port.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	gopenai "github.com/sashabaranov/go-openai"

	"github.com/edgard/murailobot/internal/common/config"
	"github.com/edgard/murailobot/internal/common/util"
	"github.com/edgard/murailobot/internal/domain/model"
	"github.com/edgard/murailobot/internal/port/ai"
	"github.com/edgard/murailobot/internal/port/store"
)

// aiService implements the ai.Service interface using OpenAI's API
type aiService struct {
	client             *gopenai.Client
	model              string
	temperature        float32
	instruction        string
	profileInstruction string
	timeout            time.Duration
	store              store.Store
	botInfo            ai.BotInfo
}

// NewAIService creates a new AI service with the provided configuration and storage.
func NewAIService(cfg *config.Config, store store.Store) (ai.Service, error) {
	if store == nil {
		return nil, errors.New("nil storage")
	}

	aiConfig := gopenai.DefaultConfig(cfg.AIToken)
	aiConfig.BaseURL = cfg.AIBaseURL

	service := &aiService{
		client:             gopenai.NewClientWithConfig(aiConfig),
		model:              cfg.AIModel,
		temperature:        cfg.AITemperature,
		instruction:        cfg.AIInstruction,
		profileInstruction: cfg.AIProfileInstruction,
		timeout:            cfg.AITimeout,
		store:              store,
	}

	return service, nil
}

// SetBotInfo configures the bot's identity information
func (s *aiService) SetBotInfo(info ai.BotInfo) error {
	if info.UserID <= 0 {
		return errors.New("invalid bot user ID")
	}

	if info.Username == "" {
		return errors.New("empty bot username")
	}

	s.botInfo = info

	return nil
}

func formatMessage(msg *model.Message) string {
	return fmt.Sprintf("[%s] UID %d: %s",
		msg.Timestamp.Format(time.RFC3339),
		msg.UserID,
		msg.Content)
}

// CreateSystemPrompt generates the system prompt for the AI model
func (s *aiService) CreateSystemPrompt(userProfiles map[int64]*model.UserProfile) string {
	// Use the bot's display name if available, otherwise fall back to username
	displayName := s.botInfo.Username
	if s.botInfo.DisplayName != "" {
		displayName = s.botInfo.DisplayName
	}

	// Create a personalized header that defines the bot's identity and expected behavior
	botIdentityHeader := fmt.Sprintf(
		"You are %s, a Telegram bot in a group chat. When someone mentions you with @%s, "+
			"your task is to respond to their message. Messages will include the @%s mention - "+
			"this is normal and expected. Always respond directly to the content of the message. "+
			"Even if the message doesn't contain a clear question, assume it's directed at you "+
			"and respond appropriately.\n\n",
		displayName, s.botInfo.Username, s.botInfo.Username)

	// Combine the identity header with the configured instruction text
	systemPrompt := botIdentityHeader + s.instruction

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
			if id == s.botInfo.UserID {
				// For the bot's own profile, use a standardized format
				botDisplayNames := s.botInfo.Username
				if s.botInfo.DisplayName != "" && s.botInfo.DisplayName != s.botInfo.Username {
					botDisplayNames = fmt.Sprintf("%s, %s", s.botInfo.DisplayName, s.botInfo.Username)
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

// GenerateResponse creates an AI-generated response to a user message
func (s *aiService) GenerateResponse(ctx context.Context, request *ai.Request) (string, error) {
	startTime := time.Now()

	// Check for nil request first to avoid nil pointer dereference
	if request == nil {
		return "", errors.New("nil request")
	}

	slog.Debug("generating AI response", "user_id", request.UserID)

	if request.UserID <= 0 {
		return "", errors.New("invalid user ID")
	}

	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return "", errors.New("empty user message")
	}

	systemPrompt := s.CreateSystemPrompt(request.UserProfiles)

	// Initialize the messages array with the system prompt
	messages := []gopenai.ChatCompletionMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	// Add the conversation history to the messages array
	// Properly identifying bot messages as "assistant" and user messages as "user"
	for _, msg := range request.RecentMessages {
		role := "user"
		if msg.UserID == s.botInfo.UserID {
			role = "assistant"
		}

		messages = append(messages, gopenai.ChatCompletionMessage{
			Role:    role,
			Content: formatMessage(msg),
		})
	}

	currentMsg := formatMessage(&model.Message{
		UserID:    request.UserID,
		Content:   request.Message,
		Timestamp: time.Now().UTC(),
	})

	messages = append(messages, gopenai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		// Estimate total tokens for logging purposes
		totalInputTokens := util.EstimateTokens(systemPrompt) + util.EstimateTokens(request.Message)
		for _, msg := range request.RecentMessages {
			totalInputTokens += util.EstimateTokens(msg.Content)
		}

		slog.Debug("sending AI request",
			"messages", len(messages),
			"tokens", totalInputTokens)
	}

	// Create a timeout context to prevent hanging on API calls
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Make API call with timeout
	apiStartTime := time.Now()
	resp, err := s.client.CreateChatCompletion(timeoutCtx, gopenai.ChatCompletionRequest{
		Model:       s.model,
		Messages:    messages,
		Temperature: s.temperature,
	})
	apiDuration := time.Since(apiStartTime)

	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}

	slog.Debug("AI response received",
		"api_duration_ms", apiDuration.Milliseconds(),
		"total_tokens", resp.Usage.TotalTokens)

	if len(resp.Choices) == 0 {
		return "", errors.New("no response choices returned")
	}

	rawResponse := resp.Choices[0].Message.Content

	result, err := util.Sanitize(rawResponse)
	if err != nil {
		return "", fmt.Errorf("failed to sanitize response: %w", err)
	}

	// Single comprehensive log with all relevant metrics
	slog.Info("AI response generated",
		"user_id", request.UserID,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"tokens", resp.Usage.TotalTokens)

	return result, nil
}

// GenerateProfiles analyzes user messages to create or update user profiles
func (s *aiService) GenerateProfiles(ctx context.Context, messages []*model.Message, existingProfiles map[int64]*model.UserProfile) (map[int64]*model.UserProfile, error) {
	startTime := time.Now()

	slog.Debug("starting profile generation",
		"messages", len(messages),
		"profiles", len(existingProfiles))

	if len(messages) == 0 {
		return nil, errors.New("no messages to analyze")
	}

	userMessages := make(map[int64][]*model.Message)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	instruction := getProfileInstruction(s.profileInstruction, s.botInfo)

	chatMessages := []gopenai.ChatCompletionMessage{
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

	// Format messages by user
	for userID, userMsgs := range userMessages {
		msgBuilder.WriteString(fmt.Sprintf("Messages from User %d:\n", userID))

		for _, msg := range userMsgs {
			msgBuilder.WriteString(fmt.Sprintf("[%s] %s\n",
				msg.Timestamp.Format(time.RFC3339),
				msg.Content))
		}

		msgBuilder.WriteString("\n")
	}

	messageContent := msgBuilder.String()

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		messageContentTokens := util.EstimateTokens(messageContent)
		slog.Debug("message content prepared",
			"users", len(userMessages),
			"tokens", messageContentTokens)
	}

	// Add the user message to the chat messages
	chatMessages = append(chatMessages, gopenai.ChatCompletionMessage{
		Role:    "user",
		Content: messageContent,
	})

	// Create a timeout context for the API call
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	apiStartTime := time.Now()
	resp, err := s.client.CreateChatCompletion(timeoutCtx, gopenai.ChatCompletionRequest{
		Model:       s.model,
		Messages:    chatMessages,
		Temperature: s.temperature,
	})
	apiDuration := time.Since(apiStartTime)

	if err != nil {
		return nil, fmt.Errorf("profile generation failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no response choices returned")
	}

	profiles, err := parseProfileResponse(resp.Choices[0].Message.Content, userMessages, existingProfiles, s.botInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profiles: %w", err)
	}

	// Consolidated log with all metrics
	slog.Info("profile generation completed",
		"duration_ms", time.Since(startTime).Milliseconds(),
		"api_ms", apiDuration.Milliseconds(),
		"profile_count", len(profiles),
		"tokens", resp.Usage.TotalTokens)

	return profiles, nil
}

func parseProfileResponse(response string, userMessages map[int64][]*model.Message, existingProfiles map[int64]*model.UserProfile, botInfo ai.BotInfo) (map[int64]*model.UserProfile, error) {
	startTime := time.Now()

	slog.Debug("parsing profile response", "response_length", len(response))

	response = strings.TrimSpace(response)
	if response == "" {
		slog.Error("empty profile response received")

		return nil, errors.New("empty profile response")
	}

	// Extract the JSON content from the response
	// AI models sometimes include explanatory text before/after the JSON
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		slog.Error("invalid JSON format in response",
			"json_start", jsonStart,
			"json_end", jsonEnd)

		return nil, errors.New("invalid JSON format in response")
	}

	jsonContent := response[jsonStart : jsonEnd+1]
	slog.Debug("extracted JSON content", "json_length", len(jsonContent))

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
	unmarshalStartTime := time.Now()
	err := json.Unmarshal([]byte(jsonContent), &profileResponse)
	unmarshalDuration := time.Since(unmarshalStartTime)

	if err != nil {
		slog.Error("failed to parse JSON response",
			"error", err,
			"unmarshal_duration_ms", unmarshalDuration.Milliseconds())

		return nil, fmt.Errorf("failed to parse profile response: %w", err)
	}

	slog.Debug("JSON unmarshaled successfully",
		"duration_ms", unmarshalDuration.Milliseconds())

	// Initialize an empty users map if none was provided in the response
	if profileResponse.Users == nil {
		slog.Warn("no users found in profile response, initializing empty map")

		profileResponse.Users = make(map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		})
	}

	updatedProfiles := make(map[int64]*model.UserProfile)
	newProfiles := 0
	updatedExistingProfiles := 0
	skippedProfiles := 0

	// Process each user profile from the response
	for userIDStr, profile := range profileResponse.Users {
		// Convert string user ID to int64
		var userID int64
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID == 0 {
			slog.Warn("invalid user ID in profile response", "user_id", userIDStr)

			skippedProfiles++

			continue
		}

		// Special handling for the bot's own profile
		// This ensures the bot always has a consistent profile
		if userID == botInfo.UserID {
			slog.Debug("handling bot's own profile", "bot_id", botInfo.UserID)

			botDisplayNames := botInfo.Username
			if botInfo.DisplayName != "" && botInfo.DisplayName != botInfo.Username {
				botDisplayNames = fmt.Sprintf("%s, %s", botInfo.DisplayName, botInfo.Username)
			}

			updatedProfiles[userID] = &model.UserProfile{
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
				slog.Debug("skipping user with no messages and no existing profile",
					"user_id", userID)

				skippedProfiles++

				continue
			}
		}

		// Check if this is a new profile or an update to an existing one
		_, isExisting := existingProfiles[userID]

		// Create updated profile for the user
		updatedProfiles[userID] = &model.UserProfile{
			UserID:          userID,
			DisplayNames:    profile.DisplayNames,
			OriginLocation:  profile.OriginLocation,
			CurrentLocation: profile.CurrentLocation,
			AgeRange:        profile.AgeRange,
			Traits:          profile.Traits,
			LastUpdated:     time.Now().UTC(),
		}

		if isExisting {
			updatedExistingProfiles++
		} else {
			newProfiles++
		}

		slog.Debug("profile created",
			"user_id", userID,
			"is_update", isExisting,
			"display_names", profile.DisplayNames)
	}

	// Only log parsing completion at DEBUG with minimal info
	slog.Debug("profile parsing completed",
		"total_profiles", len(updatedProfiles),
		"duration_ms", time.Since(startTime).Milliseconds())

	return updatedProfiles, nil
}

func getProfileInstruction(configInstruction string, botInfo ai.BotInfo) string {
	slog.Debug("generating profile instruction",
		"bot_id", botInfo.UserID,
		"bot_username", botInfo.Username)

	startTime := time.Now()

	// Create the bot identification and fixed instruction part
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

	// Combine the configured instruction with the fixed part
	fullInstruction := fmt.Sprintf("%s\n\n%s", configInstruction, botIdentificationAndFixedPart)

	// Log information about the generated instruction
	instructionLength := len(fullInstruction)
	configLength := len(configInstruction)
	fixedPartLength := len(botIdentificationAndFixedPart)

	duration := time.Since(startTime)
	slog.Debug("profile instruction generated",
		"total_length", instructionLength,
		"config_length", configLength,
		"fixed_part_length", fixedPartLength,
		"duration_ms", duration.Milliseconds())

	return fullInstruction
}
