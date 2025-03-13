// Package config manages application configuration from environment variables,
// config files, and default values.
package config

import (
	"errors"
	"time"
)

// Configuration constants.
const (
	// DefaultAITimeout defines the default timeout for AI API requests.
	DefaultAITimeout = 2 * time.Minute

	// DefaultLogLevel defines the default logging level.
	DefaultLogLevel = "info"

	// DefaultLogFormat defines the default logging format.
	DefaultLogFormat = "json"
)

// Default messages for Telegram bot responses.
const (
	DefaultWelcomeMessage       = "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation."
	DefaultNotAuthorizedMessage = "🚫 Access denied. Please contact the administrator."
	DefaultProvideMessagePrompt = "ℹ️ Please provide a message with your command."
	DefaultGeneralErrorMessage  = "❌ An error occurred. Please try again later."
	DefaultHistoryResetMessage  = "🔄 Chat history has been cleared."
)

// ErrValidation indicates a configuration validation error.
var ErrValidation = errors.New("validation error")

// defaultConfig holds the default configuration values.
var defaultConfig = map[string]any{
	"ai.base_url":    "https://api.openai.com/v1",
	"ai.model":       "gpt-4",
	"ai.temperature": 1.0,
	"ai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"ai.timeout":     DefaultAITimeout,

	"telegram.messages.welcome":         DefaultWelcomeMessage,
	"telegram.messages.not_authorized":  DefaultNotAuthorizedMessage,
	"telegram.messages.provide_message": DefaultProvideMessagePrompt,
	"telegram.messages.general_error":   DefaultGeneralErrorMessage,
	"telegram.messages.history_reset":   DefaultHistoryResetMessage,

	"log.level":  DefaultLogLevel,
	"log.format": DefaultLogFormat,
}

// Config defines the application configuration.
// Values can be set via environment variables prefixed with BOT_ (e.g., BOT_AI_TOKEN)
// or through config.yaml.
type Config struct {
	// AI API Configuration
	//
	// AIToken is the authentication token for the OpenAI API
	AIToken string `koanf:"ai.token" validate:"required"`

	// AIBaseURL is the base URL for the OpenAI API (including version path)
	AIBaseURL string `koanf:"ai.base_url" validate:"required,url"`

	// AIModel specifies which GPT model to use (e.g., "gpt-4")
	AIModel string `koanf:"ai.model" validate:"required"`

	// AITemperature controls response randomness (0.0-2.0, higher = more random)
	AITemperature float32 `koanf:"ai.temperature" validate:"required,min=0,max=2"`

	// AIInstruction provides the system message for the AI
	AIInstruction string `koanf:"ai.instruction" validate:"required"`

	// AITimeout sets maximum duration for API requests
	AITimeout time.Duration `koanf:"ai.timeout" validate:"required,min=1s,max=10m"`

	// Telegram Bot Configuration
	//
	// TelegramToken is the bot's API authentication token
	TelegramToken string `koanf:"telegram.token" validate:"required"`

	// TelegramAdminID is the Telegram user ID of the bot administrator
	TelegramAdminID int64 `koanf:"telegram.admin_id" validate:"required,gt=0"`

	// Message Templates
	//
	// These define the bot's response messages for different situations
	TelegramWelcomeMessage       string `koanf:"telegram.messages.welcome"         validate:"required"`
	TelegramNotAuthorizedMessage string `koanf:"telegram.messages.not_authorized"  validate:"required"`
	TelegramProvideMessage       string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramGeneralErrorMessage  string `koanf:"telegram.messages.general_error"   validate:"required"`
	TelegramHistoryResetMessage  string `koanf:"telegram.messages.history_reset"   validate:"required"`

	// Logging Configuration
	//
	// LogLevel sets the minimum logging level (debug|info|warn|error)
	LogLevel string `koanf:"log.level" validate:"required,oneof=debug info warn error"`

	// LogFormat specifies the log output format (json|text)
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`
}
