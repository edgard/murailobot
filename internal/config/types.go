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
	MinTemperature       = 0.0              // Minimum value for OpenAI temperature parameter
	MaxTemperature       = 2.0              // Maximum value for OpenAI temperature parameter
	MinTimeout           = time.Second      // Minimum timeout duration for API calls
	MaxTimeout           = 10 * time.Minute // Maximum timeout duration for API calls
	DefaultOpenAITimeout = 2 * time.Minute  // Default timeout for OpenAI API calls
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
	OpenAIToken string `koanf:"openai.token" validate:"required"`
	// Base URL for OpenAI API, defaults to https://api.openai.com/v1
	OpenAIBaseURL string `koanf:"openai.base_url" validate:"required,url"`
	// Model identifier (e.g., "gpt-4")
	OpenAIModel string `koanf:"openai.model" validate:"required"`
	// Temperature controls response randomness (0.0-2.0)
	OpenAITemperature float32 `koanf:"openai.temperature" validate:"required,min=0,max=2"`
	// System instruction defining assistant behavior
	OpenAIInstruction string `koanf:"openai.instruction" validate:"required,min=1"`
	// Timeout for OpenAI API calls (1s-10m)
	OpenAITimeout time.Duration `koanf:"openai.timeout" validate:"required,min=1s,max=10m"`

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
