package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/errs"
	"github.com/edgard/murailobot/internal/logging"
	"github.com/edgard/murailobot/internal/text"
	"github.com/sashabaranov/go-openai"
)

// New creates a new AI client with the provided configuration and database connection.
//
//nolint:ireturn // Interface return is intentional for better abstraction
func New(cfg *config.Config, db db.Database) (Service, error) {
	if cfg == nil {
		return nil, errs.NewValidationError("nil config", nil)
	}

	aiCfg := openai.DefaultConfig(cfg.AIToken)
	aiCfg.BaseURL = cfg.AIBaseURL

	c := &client{
		aiClient:           openai.NewClientWithConfig(aiCfg),
		model:              cfg.AIModel,
		temperature:        cfg.AITemperature,
		instruction:        cfg.AIInstruction,
		profileInstruction: cfg.AIProfileInstruction,
		timeout:            cfg.AITimeout,
		db:                 db,
	}

	return c, nil
}

// SetBotInfo sets the bot's Telegram User ID, username, and display name for profile handling.
func (c *client) SetBotInfo(uid int64, username string, displayName string) error {
	if uid <= 0 {
		return errs.NewValidationError("invalid bot user ID", nil)
	}

	if username == "" {
		return errs.NewValidationError("empty bot username", nil)
	}

	c.botUID = uid
	c.botUsername = username
	c.botDisplayName = displayName

	return nil
}

// Generate creates an AI response for a user message with context from recent messages.
func (c *client) Generate(userID int64, userMsg string, recentMessages []db.GroupMessage, userProfiles map[int64]*db.UserProfile) (string, error) {
	if userID <= 0 {
		return "", errs.NewValidationError("invalid user ID", nil)
	}

	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", errs.NewValidationError("empty user message", nil)
	}

	// Prepare system instruction with user profiles if available
	systemPrompt := c.instruction

	if len(userProfiles) > 0 {
		var profileInfo strings.Builder

		profileInfo.WriteString("\n\n## USER PROFILES\n")
		profileInfo.WriteString("Format: UID [user_id] ([display_names]) | [origin_location] | [current_location] | [age_range] | [traits]\n\n")

		// Sort user IDs for consistent order
		userIDs := make([]int64, 0, len(userProfiles))
		for id := range userProfiles {
			userIDs = append(userIDs, id)
		}

		sort.Slice(userIDs, func(i, j int) bool {
			return userIDs[i] < userIDs[j]
		})

		// Add each profile in the pipe-delimited format
		for _, id := range userIDs {
			// Add special handling for bot's profile
			if id == c.botUID {
				// Create display names by combining username and display name
				botDisplayNames := c.botUsername
				if c.botDisplayName != "" && c.botDisplayName != c.botUsername {
					botDisplayNames = fmt.Sprintf("%s, %s", c.botDisplayName, c.botUsername)
				}

				profileInfo.WriteString(fmt.Sprintf("UID %d (%s) | Internet | Internet | N/A | Group Chat Bot\n", id, botDisplayNames))

				continue
			}

			profile := userProfiles[id]
			profileInfo.WriteString(profile.FormatPipeDelimited() + "\n")
		}

		systemPrompt += profileInfo.String()
	}

	messages := make([]openai.ChatCompletionMessage, 0, messagesSliceCapacity)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// Add recent messages as context (already in chronological order from DB)
	// Note: These are previous messages only, not including the current one
	// since we modified handleGroupMessage to get messages before saving the current one
	if len(recentMessages) > 0 {
		for _, msg := range recentMessages {
			role := "user"
			// If the message is from the bot, mark it as assistant
			if msg.UserID == c.botUID {
				role = "assistant"
			}

			formattedMsg := fmt.Sprintf("[%s] UID %d: %s",
				msg.Timestamp.Format(time.RFC3339),
				msg.UserID,
				msg.Message)

			messages = append(messages, openai.ChatCompletionMessage{
				Role:    role,
				Content: formattedMsg,
			})
		}
	}

	// Add the current message with the current timestamp
	// This is the NEW message the user just sent that hasn't been saved to the DB yet
	currentTimestamp := time.Now().UTC()
	currentMsg := fmt.Sprintf("[%s] UID %d: %s",
		currentTimestamp.Format(time.RFC3339),
		userID,
		userMsg,
	)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	response, err := c.createCompletion(completionRequest{
		messages: messages,
		userID:   userID,
	})
	if err != nil {
		return "", errs.NewAPIError("failed to generate AI response", err)
	}

	return response, nil
}

// GenerateUserProfiles creates or updates user profiles based on message analysis.
func (c *client) GenerateUserProfiles(messages []db.GroupMessage, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	if len(messages) == 0 {
		return nil, errs.NewValidationError("no messages to analyze", nil)
	}

	// Group messages by user
	userMessages := make(map[int64][]db.GroupMessage)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	// Format messages for profile analysis
	chatMessages := make([]openai.ChatCompletionMessage, 0, len(messages)+extraMessageSlots)
	// Combine configurable instruction with fixed required parts
	fullInstruction := fmt.Sprintf("%s\n\n%s", c.profileInstruction, profileInstructionFixed)
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: fullInstruction,
	})

	// Build the conversation context
	var msgBuilder strings.Builder

	// Add existing profile information if available
	if len(existingProfiles) > 0 {
		msgBuilder.WriteString("Existing User Profiles:\n\n")

		// Format existing profiles as JSON for consistency
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

	// Add new messages
	msgBuilder.WriteString("New Group Chat Messages:\n\n")

	// Track total messages for logging
	totalMessages := 0

	for userID, userMsgs := range userMessages {
		msgBuilder.WriteString(fmt.Sprintf("Messages from User %d:\n", userID))

		for _, msg := range userMsgs {
			msgBuilder.WriteString(fmt.Sprintf("[%s] %s\n",
				msg.Timestamp.Format(time.RFC3339),
				msg.Message))

			totalMessages++
		}

		msgBuilder.WriteString("\n")
	}

	logging.Info("analyzing group messages for profile updates",
		"total_messages", totalMessages,
		"unique_users", len(userMessages),
		"existing_profiles", len(existingProfiles))

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: msgBuilder.String(),
	})

	response, err := c.createCompletion(completionRequest{
		messages: chatMessages,
		userID:   0, // Just for logging
	})
	if err != nil {
		return nil, errs.NewAPIError("failed to generate profiles", err)
	}

	// Parse the JSON response
	var profileResponse struct {
		Users map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		} `json:"users"`
	}

	// Trim response and validate it's not empty
	response = strings.TrimSpace(response)
	if response == "" {
		return nil, errs.NewAPIError("empty profile response from AI", nil)
	}

	// Extract JSON portion if there's extra text before or after
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		logging.Error("could not find valid JSON object in response", "response", response)

		return nil, errs.NewAPIError("invalid JSON format in profile response", nil)
	}

	jsonContent := response[jsonStart : jsonEnd+1]
	logging.Debug("extracted JSON content", "length", len(jsonContent))

	if err := json.Unmarshal([]byte(jsonContent), &profileResponse); err != nil {
		// Try to clean the JSON content - AI models sometimes output invalid JSON with comments
		// or other characters that can cause parsing to fail
		cleanedJSON := text.SanitizeJSON(jsonContent)

		if err := json.Unmarshal([]byte(cleanedJSON), &profileResponse); err != nil {
			logging.Error("json unmarshal error after cleaning",
				"error", err,
				"original", jsonContent,
				"cleaned", cleanedJSON)

			return nil, errs.NewAPIError("failed to parse profile response", err)
		}

		logging.Info("successfully parsed response after JSON cleaning")
	}

	// Initialize empty map for users if it's nil to avoid nil map errors
	if profileResponse.Users == nil {
		profileResponse.Users = make(map[string]struct {
			DisplayNames    string `json:"display_names"`
			OriginLocation  string `json:"origin_location"`
			CurrentLocation string `json:"current_location"`
			AgeRange        string `json:"age_range"`
			Traits          string `json:"traits"`
		})

		logging.Warn("empty users object in profile response, using empty map")
	}

	// Convert to UserProfile objects
	updatedProfiles := make(map[int64]*db.UserProfile)

	// Return empty map if no users found instead of nil
	if len(profileResponse.Users) == 0 {
		logging.Warn("no users in profile response")

		return updatedProfiles, nil
	}

	for userIDStr, profile := range profileResponse.Users {
		// Convert user ID string to int64
		userID := int64(0)
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil {
			logging.Warn("invalid user ID in profile response",
				"user_id", userIDStr,
				"error", err)

			continue
		}

		// Skip if user ID invalid
		if userID == 0 {
			logging.Warn("user ID is zero", "user_id_str", userIDStr)

			continue
		}

		// Add special handling for bot's profile
		if userID == c.botUID {
			logging.Debug("adding bot's own profile with special handling", "bot_uid", c.botUID)

			// Create display names by combining username and display name
			botDisplayNames := c.botUsername
			if c.botDisplayName != "" && c.botDisplayName != c.botUsername {
				botDisplayNames = fmt.Sprintf("%s, %s", c.botDisplayName, c.botUsername)
			}

			// Create a special profile for the bot
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

		// Skip if no messages exist for this user and it's not an existing profile
		if _, hasMessages := userMessages[userID]; !hasMessages {
			if _, hasProfile := existingProfiles[userID]; !hasProfile {
				logging.Warn("profile received for user with no messages and no existing profile",
					"user_id", userID)

				continue
			}
		}

		// Create or update profile
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

	logging.Info("user profiles generated",
		"profiles_created", len(updatedProfiles),
		"total_messages", totalMessages)

	return updatedProfiles, nil
}

// createCompletion handles the common logic for making API requests.
func (c *client) createCompletion(req completionRequest) (string, error) {
	var response string

	// Create context with timeout to prevent hanging API calls
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.aiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    req.messages,
		Temperature: c.temperature,
	})
	if err != nil {
		return "", errs.NewAPIError("chat completion API call failed", err)
	}

	if len(resp.Choices) == 0 {
		return "", errs.NewAPIError("no choices in response", nil)
	}

	result, err := text.Sanitize(resp.Choices[0].Message.Content)
	if err != nil {
		return "", errs.NewAPIError("failed to sanitize response", err)
	}

	response = result

	return response, nil
}
