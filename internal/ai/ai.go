package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils/text"
	"github.com/sashabaranov/go-openai"
)

// New creates a new AI client with the provided configuration and database connection.
func New(cfg *config.Config, database Database) (*Client, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	aiCfg := openai.DefaultConfig(cfg.AIToken)
	aiCfg.BaseURL = cfg.AIBaseURL

	c := &Client{
		aiClient:    openai.NewClientWithConfig(aiCfg),
		model:       cfg.AIModel,
		temperature: cfg.AITemperature,
		instruction: cfg.AIInstruction,
		timeout:     cfg.AITimeout,
		db:          database,
	}

	return c, nil
}

// Generate creates an AI response for a user message.
func (c *Client) Generate(userID int64, userName string, userMsg string) (string, error) {
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

	usernameDisplay := "unknown"
	if userName != "" {
		usernameDisplay = userName
	}

	currentMsg := fmt.Sprintf("[%s] UID %d (%s): %s",
		time.Now().Format(time.RFC3339),
		userID,
		usernameDisplay,
		userMsg,
	)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var attemptCount uint

	response, err := c.createCompletion(ctx, completionRequest{
		messages:   messages,
		userID:     userID,
		userName:   userName,
		attemptNum: &attemptCount,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate AI response: %w", err)
	}

	return response, nil
}

// GenerateUserAnalysis creates a behavioral analysis of users' messages.
func (c *Client) GenerateUserAnalysis(userID int64, userName string, messages []db.GroupMessage) (*db.UserAnalysis, error) {
	if len(messages) == 0 {
		return nil, ErrNoMessages
	}

	// Group messages by user
	userMessages := make(map[int64][]db.GroupMessage)
	userNames := make(map[int64]string)

	for _, msg := range messages {
		userMessages[msg.UserID] = append(userMessages[msg.UserID], msg)
		userNames[msg.UserID] = msg.UserName
	}

	if _, exists := userMessages[userID]; !exists {
		return nil, ErrNoUserMessages
	}

	// Format messages for behavior analysis
	chatMessages := make([]openai.ChatCompletionMessage, 0, len(messages)+extraMessageSlots)
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role: "system",
		Content: `You are a behavioral analyst analyzing chat messages from multiple users in a group. Consider:

1. Individual Analysis (for each user):
   - Communication style (formal, casual, direct)
   - Personality traits
   - Behavioral patterns
   - Word choice patterns
   - Interaction habits
   - Unique quirks
   - Mood analysis:
     * Overall emotional state
     * Mood variations
     * Emotional triggers
     * Emotional patterns

2. Group Dynamics:
   - How users interact with each other
   - Communication patterns between users
   - Group mood and atmosphere
   - Common topics or interests
   - Social dynamics and relationships

Respond with a JSON object containing a "users" object with user IDs as keys, each containing individual analyses, and a "group" object for overall dynamics.`,
	})

	// Build the conversation context
	var conversation strings.Builder

	conversation.WriteString("Group Chat Messages:\n\n")

	for userID, userMsgs := range userMessages {
		userName := userNames[userID]
		conversation.WriteString(fmt.Sprintf("Messages from %s (ID: %d):\n", userName, userID))

		for _, msg := range userMsgs {
			conversation.WriteString(fmt.Sprintf("[%s] %s\n",
				msg.Timestamp.Format("15:04"),
				msg.Message))
		}

		conversation.WriteString("\n")
	}

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: conversation.String(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var attemptCount uint

	response, err := c.createCompletion(ctx, completionRequest{
		messages:   chatMessages,
		userID:     userID,
		userName:   userName,
		attemptNum: &attemptCount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate analysis: %w", err)
	}

	// Parse the JSON response
	var analysisData struct {
		Users map[string]struct {
			CommunicationStyle string                 `json:"communicationStyle"`
			PersonalityTraits  []string               `json:"personalityTraits"`
			BehavioralPatterns []string               `json:"behavioralPatterns"`
			WordChoices        map[string]interface{} `json:"wordChoices"`
			InteractionHabits  map[string]interface{} `json:"interactionHabits"`
			Quirks             []string               `json:"quirks"`
			Mood               struct {
				Overall    string   `json:"overall"`
				Variations []string `json:"variations"`
				Triggers   []string `json:"triggers"`
				Patterns   []string `json:"patterns"`
			} `json:"mood"`
		} `json:"users"`
		Group struct {
			Dynamics     []string `json:"dynamics"`
			CommonTopics []string `json:"commonTopics"`
			Atmosphere   string   `json:"atmosphere"`
		} `json:"group"`
	}

	if err := json.Unmarshal([]byte(response), &analysisData); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJSONUnmarshal, err)
	}

	// Extract the target user's analysis
	userIDStr := strconv.FormatInt(userID, 10)

	userAnalysis, exists := analysisData.Users[userIDStr]
	if !exists {
		return nil, fmt.Errorf("%w: user %d", ErrUserNotFound, userID)
	}

	// Create and populate the user analysis
	analysis := &db.UserAnalysis{
		UserID:       userID,
		UserName:     userName,
		Date:         time.Now().UTC(),
		MessageCount: len(userMessages[userID]),
	}

	// Store the individual analysis components with error handling
	personalityTraits, err := json.Marshal(userAnalysis.PersonalityTraits)
	if err != nil {
		return nil, fmt.Errorf("%w: personality traits", ErrJSONMarshal)
	}

	analysis.PersonalityTraits = string(personalityTraits)

	behavioralPatterns, err := json.Marshal(userAnalysis.BehavioralPatterns)
	if err != nil {
		return nil, fmt.Errorf("%w: behavioral patterns", ErrJSONMarshal)
	}

	analysis.BehavioralPatterns = string(behavioralPatterns)

	wordChoices, err := json.Marshal(userAnalysis.WordChoices)
	if err != nil {
		return nil, fmt.Errorf("%w: word choices", ErrJSONMarshal)
	}

	analysis.WordChoices = string(wordChoices)

	interactionHabits, err := json.Marshal(userAnalysis.InteractionHabits)
	if err != nil {
		return nil, fmt.Errorf("%w: interaction habits", ErrJSONMarshal)
	}

	analysis.InteractionHabits = string(interactionHabits)

	quirks, err := json.Marshal(userAnalysis.Quirks)
	if err != nil {
		return nil, fmt.Errorf("%w: quirks", ErrJSONMarshal)
	}

	analysis.Quirks = string(quirks)

	mood, err := json.Marshal(userAnalysis.Mood)
	if err != nil {
		return nil, fmt.Errorf("%w: mood", ErrJSONMarshal)
	}

	analysis.Mood = string(mood)

	analysis.CommunicationStyle = userAnalysis.CommunicationStyle

	return analysis, nil
}

// createCompletion handles the common logic for making API requests with retries.
func (c *Client) createCompletion(ctx context.Context, req completionRequest) (string, error) {
	var response string

	err := retry.Do(
		func() error {
			*req.attemptNum++

			resp, err := c.aiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       c.model,
				Messages:    req.messages,
				Temperature: c.temperature,
			})
			if err != nil {
				logFields := []any{
					"error", err,
					"attempt", *req.attemptNum,
					"user_id", req.userID,
					"user_name", req.userName,
				}

				slog.Debug("completion attempt failed", logFields...)

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
		retry.Context(ctx),
		retry.OnRetry(func(n uint, _ error) {
			logFields := []any{
				"attempt", n + 1,
				"max_attempts", retryMaxAttempts,
				"user_id", req.userID,
				"user_name", req.userName,
			}

			slog.Debug("retrying request", logFields...)
		}),
	)
	if err != nil {
		return "", fmt.Errorf("failed to complete API request: %w", err)
	}

	return response, nil
}

// formatHistory converts database chat history entries into message format for the AI API.
func (c *Client) formatHistory(history []db.ChatHistory) []openai.ChatCompletionMessage {
	if len(history) == 0 {
		return nil
	}

	validMsgs := make([]db.ChatHistory, 0, len(history))

	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.ID <= 0 || msg.UserID <= 0 ||
			msg.UserMsg == "" || msg.BotMsg == "" || msg.Timestamp.IsZero() {
			continue
		}

		if strings.TrimSpace(msg.UserMsg) != "" && strings.TrimSpace(msg.BotMsg) != "" {
			validMsgs = append(validMsgs, msg)
		}
	}

	if len(validMsgs) == 0 {
		return nil
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(validMsgs)*messagesPerHistory)

	for i := len(validMsgs) - 1; i >= 0; i-- {
		msg := validMsgs[i]
		userMsg := strings.TrimSpace(msg.UserMsg)
		botMsg := strings.TrimSpace(msg.BotMsg)

		usernameDisplay := "unknown"
		if msg.UserName != "" {
			usernameDisplay = msg.UserName
		}

		formattedUserMsg := fmt.Sprintf("[%s] UID %d (%s): %s",
			msg.Timestamp.Format(time.RFC3339),
			msg.UserID,
			usernameDisplay,
			userMsg,
		)

		messages = append(messages,
			openai.ChatCompletionMessage{
				Role:    "user",
				Content: formattedUserMsg,
			},
			openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: botMsg,
			},
		)
	}

	return messages
}
