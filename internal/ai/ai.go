package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils/text"
	"github.com/sashabaranov/go-openai"
)

// New creates a new AI client with the provided configuration and database connection.
func New(cfg *config.Config, db db.Database) (*Client, error) {
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
		db:          db,
	}

	return c, nil
}

// Generate creates an AI response for a user message. It retrieves recent conversation
// history, formats the context, and makes API requests with automatic retries.
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

	var response string

	var attemptCount uint

	err = retry.Do(
		func() error {
			attemptCount++

			resp, err := c.aiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       c.model,
				Messages:    messages,
				Temperature: c.temperature,
			})
			if err != nil {
				slog.Debug("chat completion attempt failed",
					"error", err,
					"user_id", userID,
					"attempt", attemptCount)

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
			slog.Debug("retrying AI request",
				"attempt", n+1,
				"max_attempts", retryMaxAttempts,
				"user_id", userID)
		}),
	)
	if err != nil {
		return "", fmt.Errorf("AI generation retry failed: %w", err)
	}

	return response, nil
}

// formatHistory converts database chat history entries into message format for the AI API.
// It processes entries in reverse chronological order and includes only complete message pairs.
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
