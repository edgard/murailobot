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
// Configures the HTTP client with appropriate timeouts and initializes the
// client with the specified model settings.
func New(cfg *config.Config, db db.Database) (*Client, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	openAICfg := openai.DefaultConfig(cfg.OpenAIToken)
	openAICfg.BaseURL = cfg.OpenAIBaseURL
	httpClientTimeout := max(cfg.OpenAITimeout/httpClientTimeoutDivisor, minHTTPClientTimeout)
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

// Generate implements the OpenAIService interface, creating an AI response
// for a user message by retrieving conversation history, formatting messages
// with context, making API requests with retry logic, and validating responses.
func (c *Client) Generate(parentCtx context.Context, userID int64, userName string, userMsg string) (string, error) {
	select {
	case <-parentCtx.Done():
		return "", fmt.Errorf("context canceled before generation: %w", parentCtx.Err())
	default:
	}

	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", ErrEmptyUserMessage
	}

	// Create a context with timeout for database operations
	// This is a child context of the parent, so it will be canceled if the parent is canceled
	dbCtx, dbCancel := context.WithTimeout(parentCtx, chatHistoryTimeout)
	defer dbCancel()

	history, err := c.db.GetRecent(dbCtx, recentHistoryCount)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve chat history: %w", err)
	}

	// Check if parent context was canceled during history retrieval
	select {
	case <-parentCtx.Done():
		return "", fmt.Errorf("context canceled after history retrieval: %w", parentCtx.Err())
	default:
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

	// Use the parent context for the API call with retries
	return retryWithBackoff(parentCtx, func() (string, error) {
		select {
		case <-parentCtx.Done():
			return "", fmt.Errorf("context canceled before API call: %w", parentCtx.Err())
		default:
		}

		resp, err := c.openAIClient.CreateChatCompletion(parentCtx, openai.ChatCompletionRequest{
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

// formatHistory formats chat history entries into OpenAI message format,
// processing history in reverse chronological order and including only
// valid message pairs (user message + bot response).
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

// isPermanentAPIError identifies non-retryable API errors by checking
// the error message against known error types.
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

// retryWithBackoff implements exponential backoff retry logic for API calls,
// retrying operations that fail with transient errors up to retryMaxAttempts
// times with exponentially increasing delays between attempts.
func retryWithBackoff(parentCtx context.Context, op func() (string, error)) (string, error) {
	attempt := 0

	var lastErr error

	backoff := initialBackoffDuration

	for attempt < retryMaxAttempts {
		select {
		case <-parentCtx.Done():
			return "", fmt.Errorf("context canceled before retry attempt %d: %w", attempt+1, parentCtx.Err())
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
			case <-parentCtx.Done():
				timer.Stop()

				return "", fmt.Errorf("context canceled during retry backoff: %w", parentCtx.Err())
			case <-timer.C:
				backoff *= 2
			}
		}
	}

	return "", fmt.Errorf("all %d API attempts failed: %w", retryMaxAttempts, lastErr)
}
