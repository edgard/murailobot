package config

import (
	"errors"
	"time"
)

// Validation constants.
const (
	MinTemperature       = 0.0
	MaxTemperature       = 2.0
	MinTimeout           = time.Second
	MaxTimeout           = 10 * time.Minute
	DefaultOpenAITimeout = 2 * time.Minute
)

// Error definitions.
var ErrValidation = errors.New("validation error")

// Default configuration values.
var defaults = map[string]any{
	"openai.base_url":    "https://api.openai.com/v1",
	"openai.model":       "gpt-4",
	"openai.temperature": 1.0,
	"openai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"openai.timeout":     DefaultOpenAITimeout,

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
// Each field is validated using validator tags.
type Config struct {
	// OpenAI service configuration
	OpenAIToken       string        `koanf:"openai.token"       validate:"required"`
	OpenAIBaseURL     string        `koanf:"openai.base_url"    validate:"required,url"`
	OpenAIModel       string        `koanf:"openai.model"       validate:"required"`
	OpenAITemperature float32       `koanf:"openai.temperature" validate:"required,min=0,max=2"`
	OpenAIInstruction string        `koanf:"openai.instruction" validate:"required,min=1"`
	OpenAITimeout     time.Duration `koanf:"openai.timeout"     validate:"required,min=1s,max=10m"`

	// Logging Settings
	LogLevel  string `koanf:"log.level"  validate:"required,oneof=debug info warn error"`
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`

	// Telegram Settings
	TelegramToken                string `koanf:"telegram.token"                    validate:"required"`
	TelegramAdminID              int64  `koanf:"telegram.admin_id"                 validate:"required,gt=0"`
	TelegramWelcomeMessage       string `koanf:"telegram.messages.welcome"         validate:"required"`
	TelegramNotAuthorizedMessage string `koanf:"telegram.messages.not_authorized"  validate:"required"`
	TelegramProvideMessage       string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramAIErrorMessage       string `koanf:"telegram.messages.ai_error"        validate:"required"`
	TelegramGeneralErrorMessage  string `koanf:"telegram.messages.general_error"   validate:"required"`
	TelegramHistoryResetMessage  string `koanf:"telegram.messages.history_reset"   validate:"required"`
}
