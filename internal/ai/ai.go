package ai

import (
	"context"
	"encoding/json"
	"fmt"
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
func (c *client) Generate(userID int64, userMsg string) (string, error) {
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", ErrEmptyUserMessage
	}

	history, err := c.db.GetRecent(recentHistoryCount)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve chat history: %w", err)
	}

	messages := make([]openai.ChatCompletionMessage, 0, messagesSliceCapacity)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: c.instruction,
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

// GenerateGroupAnalysis creates a behavioral analysis for all users.
func (c *client) GenerateGroupAnalysis(messages []db.GroupMessage) (map[int64]*db.UserAnalysis, error) {
	if len(messages) == 0 {
		return nil, ErrNoMessages
	}

	// Group messages by user
	userMessages := make(map[int64][]db.GroupMessage)

	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
	}

	// Format messages for behavior analysis
	chatMessages := make([]openai.ChatCompletionMessage, 0, len(messages)+extraMessageSlots)
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role: "system",
		Content: `You are a behavioral analyst analyzing chat messages from multiple users in a group.
Analyze the chat messages and return a JSON object with the following exact structure:

{
  "users": {
    "<user_id>": {
      "communication_style": "Description of how the user communicates",
      "personality_traits": "Description of personality traits observed in messages",
      "behavioral_patterns": "Description of consistent behaviors shown in interactions",
      "word_choice_patterns": "Description of vocabulary, slang, and language patterns used",
      "interaction_habits": "Description of how the user engages with others",
      "unique_quirks": "Description of distinctive characteristics",
      "emotional_triggers": "Description of topics or interactions that cause emotional responses"
    }
  }
}

Ensure that:
1. You include analysis for all users.
2. The user_id in the JSON must be a string representation of the numeric ID.
3. All field names use snake_case format as shown above.
4. All fields must be plain text paragraphs without any nested objects or arrays.
5. Each description should be concise but informative, ideally 1-2 sentences.
6. Follow the exact field names shown in the example above - no additional or missing fields.`,
	})

	// Build the conversation context
	var msgBuilder strings.Builder

	msgBuilder.WriteString("Group Chat Messages:\n\n")

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

	logging.Info("analyzing group messages",
		"total_messages", totalMessages,
		"unique_users", len(userMessages))

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: msgBuilder.String(),
	})

	var attemptCount uint

	response, err := c.createCompletion(completionRequest{
		messages: chatMessages,
		// These are just for logging purposes in the retry mechanism
		userID:     0,
		attemptNum: &attemptCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate analysis: %w", err)
	}

	// Parse the JSON response
	var userAnalysisResponse struct {
		Users map[string]struct {
			CommunicationStyle string `json:"communication_style"`
			PersonalityTraits  string `json:"personality_traits"`
			BehavioralPatterns string `json:"behavioral_patterns"`
			WordChoicePatterns string `json:"word_choice_patterns"`
			InteractionHabits  string `json:"interaction_habits"`
			UniqueQuirks       string `json:"unique_quirks"`
			EmotionalTriggers  string `json:"emotional_triggers"`
		} `json:"users"`
	}

	if err := json.Unmarshal([]byte(response), &userAnalysisResponse); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJSONUnmarshal, err)
	}

	// Convert user analyses to return format
	result := make(map[int64]*db.UserAnalysis)

	for userIDStr, analysis := range userAnalysisResponse.Users {
		userID := int64(0)
		if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil {
			logging.Warn("invalid user ID in analysis response",
				"user_id", userIDStr,
				"error", err)

			continue
		}

		// Skip if no messages exist for this user
		if _, exists := userMessages[userID]; !exists {
			logging.Warn("analysis received for unknown user",
				"user_id", userID)

			continue
		}

		result[userID] = &db.UserAnalysis{
			UserID:             userID,
			Date:               time.Now().UTC(),
			CommunicationStyle: analysis.CommunicationStyle,
			PersonalityTraits:  analysis.PersonalityTraits,
			BehavioralPatterns: analysis.BehavioralPatterns,
			WordChoicePatterns: analysis.WordChoicePatterns,
			InteractionHabits:  analysis.InteractionHabits,
			UniqueQuirks:       analysis.UniqueQuirks,
			EmotionalTriggers:  analysis.EmotionalTriggers,
			MessageCount:       len(userMessages[userID]),
		}
	}

	logging.Info("group analysis completed",
		"users_analyzed", len(result),
		"total_messages", totalMessages)

	return result, nil
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
