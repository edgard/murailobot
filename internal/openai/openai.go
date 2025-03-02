package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/utils"
	"github.com/sashabaranov/go-openai"
)

// New creates a new OpenAI client instance with the provided configuration.
// It configures the underlying HTTP client with appropriate timeouts and
// initializes the client with the specified model settings.
//
// Parameters:
//   - cfg: Configuration containing API settings and model parameters
//   - db: Database interface for conversation history management
//
// Returns an error if configuration is nil or invalid.
func New(cfg *config.Config, db db.Database) (*Client, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	openAICfg := openai.DefaultConfig(cfg.OpenAIToken)
	openAICfg.BaseURL = cfg.OpenAIBaseURL
	httpClientTimeout := max(cfg.OpenAITimeout/HTTPClientTimeoutDivisor, MinHTTPClientTimeout)
	openAICfg.HTTPClient = &http.Client{
		Timeout: httpClientTimeout,
	}

	c := &Client{
		openAIClient: openai.NewClientWithConfig(openAICfg),
		model:        cfg.OpenAIModel,
		temperature:  cfg.OpenAITemperature,
		instruction:  cfg.OpenAIInstruction,
		db:           db,
		timeout:      cfg.OpenAITimeout,
	}

	return c, nil
}

// Generate implements the OpenAIService interface.
// It creates an AI response for a user message by:
//  1. Retrieving recent conversation history
//  2. Formatting messages with user context
//  3. Making API request with retry logic
//  4. Sanitizing and validating the response
//
// The function includes conversation history to maintain context and
// formats messages with timestamps and user information.
func (c *Client) Generate(ctx context.Context, userID int64, userName string, userMsg string) (string, error) {
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", ErrEmptyUserMessage
	}

	historyCtx, cancel := context.WithTimeout(ctx, ChatHistoryTimeout)
	defer cancel()

	history, err := c.db.GetRecent(historyCtx, RecentHistoryCount)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve chat history: %w", err)
	}

	messages := make([]openai.ChatCompletionMessage, 0, MessagesSliceCapacity)
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
		resp, err := c.openAIClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: c.temperature,
		})
		if err != nil {
			return "", fmt.Errorf("chat completion failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", ErrNoChoices
		}

		response := utils.Sanitize(resp.Choices[0].Message.Content)
		if response == "" {
			return "", ErrEmptyResponse
		}

		return response, nil
	})
}

// formatHistory formats chat history entries into OpenAI message format.
// It processes the history in reverse chronological order and includes
// only valid message pairs (user message + bot response).
//
// The function adds context to user messages by including:
//   - Timestamp in RFC3339 format
//   - User ID for tracking
//   - Username for display
//   - Original message content
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

	messages := make([]openai.ChatCompletionMessage, 0, len(validMsgs)*MessagesPerHistory)

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

// isPermanentAPIError identifies non-retryable API errors by checking
// the error message against known error types. These errors indicate
// issues that won't be resolved by retrying (e.g., invalid API key).
func isPermanentAPIError(err error) (bool, error) {
	if err == nil {
		return false, nil
	}

	errStr := err.Error()
	for _, errType := range InvalidRequestErrors {
		if strings.Contains(errStr, errType) {
			return true, fmt.Errorf("permanent API error: %w", err)
		}
	}

	return false, err
}

// retryWithBackoff implements exponential backoff retry logic for API calls.
// It will retry operations that fail with transient errors up to RetryMaxAttempts
// times, with exponentially increasing delays between attempts.
//
// The function handles:
//   - Context cancellation
//   - Permanent vs transient errors
//   - Exponential backoff timing
//   - Maximum retry attempts
func retryWithBackoff(ctx context.Context, op func() (string, error)) (string, error) {
	attempt := 0

	var lastErr error

	backoff := InitialBackoffDuration

	for attempt < RetryMaxAttempts {
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

		if attempt < RetryMaxAttempts {
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

	return "", fmt.Errorf("all %d API attempts failed: %w", RetryMaxAttempts, lastErr)
}
