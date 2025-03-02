package config

import (
	"errors"
	"time"
)

// Validation constants.
const (
	MinTemperature   = 0.0
	MaxTemperature   = 2.0
	MinTimeout       = time.Second
	MaxTimeout       = 10 * time.Minute
	DefaultAITimeout = 2 * time.Minute
)

// Error definitions.
var ErrValidation = errors.New("validation error")

// Default configuration values.
var defaults = map[string]any{
	"ai.base_url":    "https://api.openai.com/v1",
	"ai.model":       "gpt-4", // Fixed model name
	"ai.temperature": 1.0,
	"ai.instruction": "You are a helpful assistant focused on providing clear and accurate responses.",
	"ai.timeout":     DefaultAITimeout,

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
	// AI service configuration
	AIToken       string        `koanf:"ai.token"       validate:"required"`
	AIBaseURL     string        `koanf:"ai.base_url"    validate:"required,url"`
	AIModel       string        `koanf:"ai.model"       validate:"required"`
	AITemperature float32       `koanf:"ai.temperature" validate:"required,min=0,max=2"`
	AIInstruction string        `koanf:"ai.instruction" validate:"required,min=1"`
	AITimeout     time.Duration `koanf:"ai.timeout"     validate:"required,min=1s,max=10m"`

	// Logging Settings
	LogLevel  string `koanf:"log.level"  validate:"required,oneof=debug info warn error"`
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`

	// Telegram Settings
	TelegramToken          string `koanf:"telegram.token"                    validate:"required"`
	TelegramAdminID        int64  `koanf:"telegram.admin_id"                 validate:"required,gt=0"`
	TelegramWelcomeMessage string `koanf:"telegram.messages.welcome"         validate:"required"`
	TelegramNotAuthMessage string `koanf:"telegram.messages.not_authorized"  validate:"required"`
	TelegramProvideMessage string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramAIErrorMessage string `koanf:"telegram.messages.ai_error"        validate:"required"`
	TelegramGeneralError   string `koanf:"telegram.messages.general_error"   validate:"required"`
	TelegramHistoryReset   string `koanf:"telegram.messages.history_reset"   validate:"required"`
}
