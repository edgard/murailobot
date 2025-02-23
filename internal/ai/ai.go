// Package ai provides OpenAI API integration with circuit breaking and retries.
package ai

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
	"github.com/sony/gobreaker"
)

const componentName = "ai"

// Non-retryable OpenAI API errors that indicate permanent failures
var invalidRequestErrors = []string{
	"invalid_request_error",
	"context_length_exceeded",
	"rate_limit_exceeded",
	"invalid_api_key",
	"organization_not_found",
}

type client struct {
	completionSvc CompletionService
	model         string
	temperature   float32
	topP          float32
	instruction   string
	db            db.Database
	policy        *utils.TextPolicy
	breaker       *utils.CircuitBreaker
}

func New(cfg *config.AIConfig, db db.Database) (Service, error) {
	if cfg == nil {
		return nil, utils.NewError(componentName, utils.ErrInvalidConfig, "configuration is nil", utils.CategoryConfig, nil)
	}

	config := openai.DefaultConfig(cfg.Token)
	config.BaseURL = cfg.BaseURL

	config.HTTPClient = &http.Client{
		Timeout: cfg.Timeout,
	}

	breaker := utils.NewCircuitBreaker(utils.CircuitBreakerConfig{
		Name: "ai-api",
		OnStateChange: func(name string, from, to utils.CircuitState) {
			utils.WriteInfoLog(componentName, "AI API circuit breaker state changed",
				utils.KeyName, name,
				utils.KeyFrom, from.String(),
				utils.KeyTo, to.String(),
				utils.KeyType, "circuit_breaker",
				utils.KeyAction, "state_change")
		},
	})

	c := &client{
		completionSvc: openai.NewClientWithConfig(config),
		model:         cfg.Model,
		temperature:   cfg.Temperature,
		topP:          cfg.TopP,
		instruction:   cfg.Instruction,
		db:            db,
		policy:        utils.NewTelegramTextPolicy(),
		breaker:       breaker,
	}

	utils.WriteInfoLog(componentName, "AI client initialized",
		utils.KeyType, "openai",
		utils.KeyAction, "initialize",
		utils.KeyName, cfg.BaseURL,
		"model_config", map[string]interface{}{
			"model":       cfg.Model,
			"temperature": cfg.Temperature,
			"top_p":       cfg.TopP,
			"timeout":     cfg.Timeout,
		})
	return c, nil
}

func (c *client) SanitizeResponse(response string) string {
	return c.policy.SanitizeText(response)
}

// formatChatHistory processes chat history in reverse chronological order,
// validates messages, and formats them with metadata for the AI context
func (c *client) formatChatHistory(history []db.ChatHistory) []openai.ChatCompletionMessage {
	if len(history) == 0 {
		return nil
	}

	validMsgs := make([]db.ChatHistory, 0, len(history))

	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]

		if msg.ID <= 0 || msg.UserID <= 0 ||
			msg.UserMsg == "" || msg.BotMsg == "" || msg.Timestamp.IsZero() {
			utils.WriteWarnLog(componentName, "skipping invalid message in history",
				utils.KeyRequestID, msg.ID,
				utils.KeyUserID, msg.UserID,
				utils.KeyType, "chat_history",
				utils.KeyAction, "validate_message",
				utils.KeyReason, "missing required fields",
				"validation", map[string]bool{
					"has_id":        msg.ID > 0,
					"has_user_id":   msg.UserID > 0,
					"has_user_msg":  msg.UserMsg != "",
					"has_bot_msg":   msg.BotMsg != "",
					"has_timestamp": !msg.Timestamp.IsZero(),
				})
			continue
		}

		validMsgs = append(validMsgs, msg)
	}

	if len(validMsgs) == 0 {
		utils.WriteWarnLog(componentName, "no valid messages in history",
			utils.KeyCount, len(history),
			utils.KeyType, "chat_history",
			utils.KeyAction, "validate_history",
			utils.KeyReason, "all messages invalid")
		return nil
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(validMsgs)*2)

	for i := len(validMsgs) - 1; i >= 0; i-- {
		msg := validMsgs[i]
		userMsg := strings.TrimSpace(msg.UserMsg)
		botMsg := strings.TrimSpace(msg.BotMsg)

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

	utils.WriteDebugLog(componentName, "formatted chat history",
		utils.KeyType, "chat_history",
		utils.KeyAction, "format_history",
		"stats", map[string]int{
			"total_messages":  len(history),
			"valid_messages":  len(validMsgs),
			"message_entries": len(messages),
		})

	return messages
}

func (c *client) GenerateResponse(ctx context.Context, userID int64, userName string, userMsg string) (string, error) {
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return "", utils.NewError(componentName, utils.ErrValidation, "user message is empty", utils.CategoryValidation, nil)
	}

	utils.WriteDebugLog(componentName, "generating AI response",
		utils.KeyType, "openai",
		utils.KeyAction, "generate_response",
		utils.KeyUserID, userID,
		utils.KeyName, userName,
		utils.KeySize, len(userMsg),
		"model_params", map[string]interface{}{
			"model":       c.model,
			"temperature": c.temperature,
			"top_p":       c.topP,
		})

	retryConfig := utils.DefaultRetryConfig()

	var history []db.ChatHistory
	err := utils.WithRetry(ctx, func(ctx context.Context) error {
		var err error
		history, err = c.db.GetRecentChatHistory(ctx, 10)
		return err
	}, retryConfig)

	if err != nil {
		utils.WriteWarnLog(componentName, "failed to get chat history",
			utils.KeyError, err.Error(),
			utils.KeyUserID, userID,
			utils.KeyType, "chat_history",
			utils.KeyAction, "get_history",
			utils.KeyReason, "will continue without history")
		history = make([]db.ChatHistory, 0)
	}

	messages := make([]openai.ChatCompletionMessage, 0, 21)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: c.instruction,
	})

	if len(history) > 0 {
		historyMsgs := c.formatChatHistory(history)
		if len(historyMsgs) > 0 {
			messages = append(messages, historyMsgs...)
		}
	}

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

	var response string
	err = c.breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			resp, err := c.completionSvc.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       c.model,
				Messages:    messages,
				Temperature: c.temperature,
				TopP:        c.topP,
			})

			if err != nil {
				errStr := err.Error()
				for _, errType := range invalidRequestErrors {
					if strings.Contains(errStr, errType) {
						return utils.NewError(componentName, utils.ErrValidation, "permanent API error", utils.CategoryValidation, err)
					}
				}
				return err
			}

			if len(resp.Choices) == 0 {
				return utils.NewError(componentName, utils.ErrAPI, "no response choices available", utils.CategoryExternal, nil)
			}

			response = c.SanitizeResponse(resp.Choices[0].Message.Content)
			if response == "" {
				return utils.NewError(componentName, utils.ErrAPI, "empty response after sanitization", utils.CategoryExternal, nil)
			}
			return nil
		}, retryConfig)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return "", utils.NewError(componentName, utils.ErrAPI, "circuit breaker is open", utils.CategoryExternal, err)
		}
		return "", utils.NewError(componentName, utils.ErrAPI, "AI operation failed", utils.CategoryExternal, err)
	}

	return response, nil
}
