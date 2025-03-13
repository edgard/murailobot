package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils/logging"
	"github.com/edgard/murailobot/internal/utils/text"
	timeformats "github.com/edgard/murailobot/internal/utils/time"
	"github.com/sashabaranov/go-openai"
)

// New creates a new AI client with the provided configuration and database connection.
//
//nolint:ireturn // Interface return is intentional for better abstraction
func New(cfg *config.Config, db database) (Service, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	aiCfg := openai.DefaultConfig(cfg.AIToken)
	aiCfg.BaseURL = cfg.AIBaseURL

	c := &client{
		aiClient:    openai.NewClientWithConfig(aiCfg),
		model:       cfg.AIModel,
		temperature: cfg.AITemperature,
		instruction: cfg.AIInstruction,
		timeout:     cfg.AITimeout,
		db:          db,
	}

	return c, nil
}

// Generate creates an AI response for a user message.
func (c *client) Generate(userID int64, userMsg string, userProfiles map[int64]*db.UserProfile) (string, error) {
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", ErrEmptyUserMessage
	}

	history, err := c.db.GetRecent(recentHistoryCount)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve chat history: %w", err)
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

	if len(history) > 0 {
		historyMsgs := c.formatHistory(history)
		if len(historyMsgs) > 0 {
			messages = append(messages, historyMsgs...)
		}
	}

	currentMsg := fmt.Sprintf("[%s] UID %d: %s",
		time.Now().Format(timeformats.FullTimestamp),
		userID,
		userMsg,
	)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	var attemptCount uint

	response, err := c.createCompletion(completionRequest{
		messages:   messages,
		userID:     userID,
		attemptNum: &attemptCount,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate AI response: %w", err)
	}

	return response, nil
}

// GenerateUserProfiles creates or updates user profiles based on message analysis.
func (c *client) GenerateUserProfiles(messages []db.GroupMessage, existingProfiles map[int64]*db.UserProfile) (map[int64]*db.UserProfile, error) {
	if len(messages) == 0 {
		return nil, ErrNoMessages
	}

	// Group messages by user
	userMessages := make(map[int64][]db.GroupMessage)
	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	// Format messages for profile analysis
	chatMessages := make([]openai.ChatCompletionMessage, 0, len(messages)+extraMessageSlots)
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role: "system",
		Content: `You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.
Your task is to analyze chat messages and build detailed psychological profiles of users.

When analyzing messages, consider:
1. Language patterns, word choice, and communication style
2. Emotional expressions and reactions to different topics
3. Recurring themes or topics in their communications
4. Interaction patterns with other users
5. Cultural references and personal details they reveal

Analyze the messages and return a JSON object with the following structure:

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
}

If existing profile information is provided, use it as a base and only update or add information when you have new evidence from the messages.

Important: Be analytical, perceptive, and detailed in your assessment while avoiding assumptions without evidence.
Respond ONLY with the JSON object and no additional text or explanation.`,
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
				msg.Timestamp.Format(timeformats.FullTimestamp),
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

	var attemptCount uint

	response, err := c.createCompletion(completionRequest{
		messages:   chatMessages,
		userID:     0, // Just for logging
		attemptNum: &attemptCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate profiles: %w", err)
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

	if err := json.Unmarshal([]byte(response), &profileResponse); err != nil {
		logging.Error("failed to parse JSON response", "error", err, "response", response)

		return nil, fmt.Errorf("%w: %w", ErrJSONUnmarshal, err)
	}

	// Convert to UserProfile objects
	updatedProfiles := make(map[int64]*db.UserProfile)

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

		// Skip if no messages exist for this user and it's not an existing profile
		if _, hasMessages := userMessages[userID]; !hasMessages {
			if _, hasProfile := existingProfiles[userID]; !hasProfile {
				logging.Warn("profile received for user with no messages and no existing profile",
					"user_id", userID)

				continue
			}
		}

		// Calculate message count
		messageCount := 0
		if msgs, exists := userMessages[userID]; exists {
			messageCount = len(msgs)
		}

		// Add existing message count if profile exists
		if existingProfile, exists := existingProfiles[userID]; exists {
			messageCount += existingProfile.MessageCount
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
			MessageCount:    messageCount,
		}
	}

	logging.Info("user profiles generated",
		"profiles_created", len(updatedProfiles),
		"total_messages", totalMessages)

	return updatedProfiles, nil
}

// createCompletion handles the common logic for making API requests with retries.
func (c *client) createCompletion(req completionRequest) (string, error) {
	var response string

	err := retry.Do(
		func() error {
			*req.attemptNum++

			resp, err := c.aiClient.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
				Model:       c.model,
				Messages:    req.messages,
				Temperature: c.temperature,
			})
			if err != nil {
				logFields := []any{
					"error", err,
					"attempt", *req.attemptNum,
					"user_id", req.userID,
				}

				logging.Debug("completion attempt failed", logFields...)

				return fmt.Errorf("chat completion API call failed: %w", err)
			}

			if len(resp.Choices) == 0 {
				return ErrNoChoices
			}

			result := text.Sanitize(resp.Choices[0].Message.Content)
			if result == "" {
				return ErrEmptyResponse
			}

			response = result

			return nil
		},
		retry.Attempts(retryMaxAttempts),
		retry.Delay(initialBackoffDuration),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, _ error) {
			logFields := []any{
				"attempt", n + 1,
				"max_attempts", retryMaxAttempts,
				"user_id", req.userID,
			}

			logging.Debug("retrying request", logFields...)
		}),
	)
	if err != nil {
		return "", fmt.Errorf("failed to complete API request: %w", err)
	}

	return response, nil
}

// formatHistory converts database chat history entries into message format for the AI API.
func (c *client) formatHistory(history []db.ChatHistory) []openai.ChatCompletionMessage {
	if len(history) == 0 {
		return nil
	}

	validChatHistory := make([]db.ChatHistory, 0, len(history))

	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.ID <= 0 || msg.UserID <= 0 || msg.Timestamp.IsZero() {
			continue
		}

		trimmedUserMsg := strings.TrimSpace(msg.UserMsg)
		trimmedBotMsg := strings.TrimSpace(msg.BotMsg)

		if trimmedUserMsg != "" && trimmedBotMsg != "" {
			validChatHistory = append(validChatHistory, msg)
		}
	}

	if len(validChatHistory) == 0 {
		return nil
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(validChatHistory)*messagesPerHistory)

	for i := len(validChatHistory) - 1; i >= 0; i-- {
		msg := validChatHistory[i]
		userMsg := strings.TrimSpace(msg.UserMsg)
		botMsg := strings.TrimSpace(msg.BotMsg)

		formattedMsg := fmt.Sprintf("[%s] UID %d: %s",
			msg.Timestamp.Format(timeformats.FullTimestamp),
			msg.UserID,
			userMsg,
		)

		messages = append(messages,
			openai.ChatCompletionMessage{
				Role:    "user",
				Content: formattedMsg,
			},
			openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: botMsg,
			},
		)
	}

	return messages
}
