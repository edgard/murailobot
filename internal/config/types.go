// Package config provides configuration management for the Telegram bot application.
// It handles loading and validating configuration from multiple sources including
// environment variables, configuration files, and default values.
package config

import (
	"errors"
	"time"
)

// Validation constants define the acceptable ranges for various configuration parameters.
const (
	defaultOpenAITimeout = 2 * time.Minute // Default timeout for OpenAI API calls
)

// ErrValidation is returned when configuration validation fails.
var ErrValidation = errors.New("validation error")

// Default configuration values used when no override is provided.
// These values can be overridden through config.yaml or environment variables.
var defaults = map[string]any{
	"openai.base_url":    "https://api.openai.com/v1",
	"openai.model":       "gpt-4",
	"openai.temperature": 1.0,
	"openai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"openai.timeout":     defaultOpenAITimeout,

	"telegram.messages.welcome":         "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation.",
	"telegram.messages.not_authorized":  "🚫 Access denied. Please contact the administrator.",
	"telegram.messages.provide_message": "ℹ️ Please provide a message with your command.",
	"telegram.messages.ai_error":        "🤖 Unable to process request. Please try again.",
	"telegram.messages.general_error":   "❌ An error occurred. Please try again later.",
	"telegram.messages.history_reset":   "🔄 Chat history has been cleared.",

	"log.level":  "info",
	"log.format": "json",
}

// Config defines the complete application configuration.
// Configuration can be provided through environment variables prefixed with BOT_
// or through a config.yaml file. Environment variables take precedence over
// config file values.
//
// Example environment variables:
//
//	BOT_OPENAI_TOKEN=sk-xxx
//	BOT_TELEGRAM_TOKEN=123456:xxx
//	BOT_TELEGRAM_ADMIN_ID=123456789
type Config struct {
	// OpenAI service configuration
	OpenAIToken       string        `koanf:"openai.token"       validate:"required"`                // API token for OpenAI service authentication
	OpenAIBaseURL     string        `koanf:"openai.base_url"    validate:"required,url"`            // Base URL for OpenAI API, defaults to https://api.openai.com/v1
	OpenAIModel       string        `koanf:"openai.model"       validate:"required"`                // Model identifier (e.g., "gpt-4")
	OpenAITemperature float32       `koanf:"openai.temperature" validate:"required,min=0,max=2"`    // Temperature controls response randomness (0.0-2.0)
	OpenAIInstruction string        `koanf:"openai.instruction" validate:"required,min=1"`          // System instruction defining assistant behavior
	OpenAITimeout     time.Duration `koanf:"openai.timeout"     validate:"required,min=1s,max=10m"` // Timeout for OpenAI API calls (1s-10m)

	// Telegram Settings
	TelegramToken                string `koanf:"telegram.token"                    validate:"required"`      // API token for Telegram bot authentication
	TelegramAdminID              int64  `koanf:"telegram.admin_id"                 validate:"required,gt=0"` // User ID of the Telegram admin
	TelegramWelcomeMessage       string `koanf:"telegram.messages.welcome"         validate:"required"`      // Message sent when a user starts the bot
	TelegramNotAuthorizedMessage string `koanf:"telegram.messages.not_authorized"  validate:"required"`      // Message sent to unauthorized users
	TelegramProvideMessage       string `koanf:"telegram.messages.provide_message" validate:"required"`      // Message sent when user sends command without text
	TelegramAIErrorMessage       string `koanf:"telegram.messages.ai_error"        validate:"required"`      // Message sent when AI service returns an error
	TelegramGeneralErrorMessage  string `koanf:"telegram.messages.general_error"   validate:"required"`      // Message sent on general system errors
	TelegramHistoryResetMessage  string `koanf:"telegram.messages.history_reset"   validate:"required"`      // Message sent when chat history is reset

	// Logging Settings
	LogLevel  string `koanf:"log.level"  validate:"required,oneof=debug info warn error"` // Log level (debug, info, warn, error)
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`             // Log output format (json, text)
}
