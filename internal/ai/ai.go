package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/resilience"
	"github.com/edgard/murailobot/internal/sanitize"
	"github.com/sashabaranov/go-openai"
)

type AI struct {
	client      *openai.Client
	model       string
	temperature float32
	topP        float32
	instruction string
	db          db.Database
	policy      *sanitize.Policy
	breaker     *resilience.CircuitBreaker
}

// New creates a new AI client instance
func New(cfg *Config, db db.Database) (*AI, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: configuration is nil", ErrAI)
	}

	// Validate base URL
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base URL: %v", ErrAI, err)
	}
	if baseURL.Scheme != "https" {
		return nil, fmt.Errorf("%w: base URL must use HTTPS", ErrAI)
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("%w: base URL must include a host", ErrAI)
	}

	config := openai.DefaultConfig(cfg.Token)
	config.BaseURL = cfg.BaseURL

	config.HTTPClient = &http.Client{
		Timeout: cfg.Timeout,
	}

	breaker := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:          "openai-api",
		MaxFailures:   5,
		Timeout:       cfg.Timeout,
		HalfOpenLimit: 1,
		ResetInterval: cfg.Timeout * 2,
		OnStateChange: func(name string, from, to resilience.CircuitState) {
			slog.Info("OpenAI API circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	ai := &AI{
		client:      openai.NewClientWithConfig(config),
		model:       cfg.Model,
		temperature: cfg.Temperature,
		topP:        cfg.TopP,
		instruction: cfg.Instruction,
		db:          db,
		policy:      sanitize.NewTelegramPolicy(),
		breaker:     breaker,
	}

	slog.Info("OpenAI client initialized",
		"model", cfg.Model,
		"base_url", cfg.BaseURL,
		"temperature", cfg.Temperature,
		"top_p", cfg.TopP,
		"timeout", cfg.Timeout,
	)
	return ai, nil
}

// SanitizeResponse sanitizes the response text for Telegram messages
func (ai *AI) SanitizeResponse(response string) string {
	return ai.policy.SanitizeText(response)
}

func (ai *AI) convertHistoryToCompletionMessages(history []db.ChatHistory) []openai.ChatCompletionMessage {
	if len(history) == 0 {
		return nil
	}

	validMsgs := make([]db.ChatHistory, 0, len(history))

	// Filter and validate messages in reverse order (newest first)
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]

		// Validate message integrity
		if msg.ID <= 0 || msg.UserID <= 0 ||
			msg.UserMsg == "" || msg.BotMsg == "" || msg.Timestamp.IsZero() {
			slog.Warn("skipping invalid message in history",
				"message_id", msg.ID,
				"user_id", msg.UserID,
				"reason", "missing required fields",
				"has_user_msg", msg.UserMsg != "",
				"has_bot_msg", msg.BotMsg != "",
				"has_timestamp", !msg.Timestamp.IsZero(),
			)
			continue
		}

		validMsgs = append(validMsgs, msg)
	}

	if len(validMsgs) == 0 {
		slog.Warn("no valid messages in history", "total_messages", len(history))
		return nil
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(validMsgs)*2)

	// Process valid messages in chronological order
	for i := len(validMsgs) - 1; i >= 0; i-- {
		msg := validMsgs[i]
		// Trim messages to ensure no leading/trailing whitespace
		userMsg := strings.TrimSpace(msg.UserMsg)
		botMsg := strings.TrimSpace(msg.BotMsg)

		// Format user message with timestamp and user info
		formattedUserMsg := fmt.Sprintf("[%s] UID %d (%s): %s",
			msg.Timestamp.Format(time.RFC3339),
			msg.UserID,
			msg.UserName,
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

	slog.Debug("formatted chat history",
		"total_messages", len(history),
		"valid_messages", len(validMsgs),
		"total_entries", len(messages),
	)

	return messages
}

func (ai *AI) GenerateResponse(ctx context.Context, userID int64, userName string, userMsg string) (string, error) {
	if ai.instruction == "" {
		return "", fmt.Errorf("%w: system instruction is empty", ErrAI)
	}

	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", fmt.Errorf("%w: user message is empty", ErrAI)
	}

	slog.Debug("generating AI response",
		"model", ai.model,
		"message_length", len(userMsg),
		"temperature", ai.temperature,
		"top_p", ai.topP,
	)

	retryConfig := resilience.RetryConfig{
		MaxAttempts:     5,
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     10 * time.Second,
		Multiplier:      1.5,
		RandomFactor:    0.1, // Add jitter for distributed systems
	}

	// Get last 10 messages from chat history with retry
	var history []db.ChatHistory
	err := resilience.WithRetry(ctx, func(ctx context.Context) error {
		var err error
		history, err = ai.db.GetRecentChatHistory(ctx, 10)
		return err
	}, retryConfig) // Use optimized retry config

	if err != nil {
		// Log warning but continue without history
		slog.Warn("failed to get chat history",
			"error", err,
			"user_id", userID,
		)
		// Initialize empty history to continue without past context
		history = make([]db.ChatHistory, 0)
	}

	messages := make([]openai.ChatCompletionMessage, 0, 21) // Pre-allocate for system + history + user message
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: ai.instruction,
	})

	// Add history messages if available
	if len(history) > 0 {
		historyMsgs := ai.convertHistoryToCompletionMessages(history)
		if len(historyMsgs) > 0 {
			messages = append(messages, historyMsgs...)
		}
	}

	// Add current user message with timestamp
	currentMsg := fmt.Sprintf("[%s] UID %d (%s): %s",
		time.Now().Format(time.RFC3339),
		userID,
		userName,
		userMsg,
	)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: currentMsg,
	})

	// Execute request through circuit breaker with retry
	var response string
	err = ai.breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.WithRetry(ctx, func(ctx context.Context) error {
			resp, err := ai.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       ai.model,
				Messages:    messages,
				Temperature: ai.temperature,
				TopP:        ai.topP,
			})

			if err != nil {
				errStr := err.Error()
				// Check for non-retryable errors
				for _, errType := range invalidRequestErrors {
					if strings.Contains(errStr, errType) {
						return fmt.Errorf("permanent error: %v", err)
					}
				}
				return err // Retryable error
			}

			if len(resp.Choices) == 0 {
				return fmt.Errorf("%w: no response choices available", ErrAI)
			}

			response = ai.SanitizeResponse(resp.Choices[0].Message.Content)
			if response == "" {
				return fmt.Errorf("%w: empty response after sanitization", ErrAI)
			}

			// Validate response length
			if len(response) > 4096 {
				slog.Warn("response exceeded maximum length, truncating",
					"original_length", len(response),
					"truncated_length", 4096,
				)
				response = response[:4093] + "..."
			}

			return nil
		}, retryConfig) // Use optimized retry config
	})

	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return "", fmt.Errorf("%w: circuit breaker is open", ErrAI)
		}
		return "", fmt.Errorf("%w: %v", ErrAI, err)
	}

	return response, nil
}
