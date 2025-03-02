package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
	"github.com/sashabaranov/go-openai"
)

func New(cfg *config.Config, db db.Database) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("configuration is nil")
	}

	config := openai.DefaultConfig(cfg.AIToken)
	config.BaseURL = cfg.AIBaseURL
	httpClientTimeout := max(cfg.AITimeout/httpClientTimeoutDivisor, minHTTPClientTimeout)
	config.HTTPClient = &http.Client{
		Timeout: httpClientTimeout,
	}

	c := &Client{
		openaiClient: openai.NewClientWithConfig(config),
		model:        cfg.AIModel,
		temperature:  cfg.AITemperature,
		instruction:  cfg.AIInstruction,
		db:           db,
		timeout:      cfg.AITimeout,
	}

	return c, nil
}

// Identifies permanent API errors.
func isPermanentAPIError(err error) (bool, error) {
	if err == nil {
		return false, nil
	}

	errStr := err.Error()
	for _, errType := range invalidRequestErrors {
		if strings.Contains(errStr, errType) {
			return true, fmt.Errorf("permanent API error: %w", err)
		}
	}

	return false, err
}

// Retries operation with exponential backoff.
func retryWithBackoff(ctx context.Context, op func() (string, error)) (string, error) {
	attempt := 0
	var lastErr error
	backoff := initialBackoffDuration

	for attempt < retryMaxAttempts {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context error: %w", ctx.Err())
		default:
		}

		result, err := op()
		if err == nil {
			return result, nil
		}

		isPermanent, wrappedErr := isPermanentAPIError(err)
		if isPermanent {
			return "", wrappedErr
		}

		lastErr = err
		attempt++

		if attempt < retryMaxAttempts {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()

				return "", fmt.Errorf("context error: %w", ctx.Err())
			case <-timer.C:
				backoff *= 2
			}
		}
	}

	return "", fmt.Errorf("all %d API attempts failed: %w", retryMaxAttempts, lastErr)
}

// Formats chat history for AI context.
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

	messages := make([]openai.ChatCompletionMessage, 0, len(validMsgs)*2)
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

func (c *Client) Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error) {
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", errors.New("user message is empty")
	}

	historyCtx, cancel := context.WithTimeout(ctx, chatHistoryTimeout)
	defer cancel()

	history, err := c.db.GetRecent(historyCtx, recentHistoryCount)
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

	return retryWithBackoff(ctx, func() (string, error) {
		resp, err := c.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: c.temperature,
		})
		if err != nil {
			return "", fmt.Errorf("chat completion failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", errors.New("no response choices available")
		}

		response := utils.Sanitize(resp.Choices[0].Message.Content)
		if response == "" {
			return "", errors.New("empty response after sanitization")
		}

		return response, nil
	})
}
