package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config contains app settings
type Config struct {
	// AI Settings
	AIToken       string        `koanf:"ai.token" validate:"required"`
	AIBaseURL     string        `koanf:"ai.base_url" validate:"required,url"`
	AIModel       string        `koanf:"ai.model" validate:"required"`
	AITemperature float32       `koanf:"ai.temperature" validate:"required,min=0,max=2"`
	AIInstruction string        `koanf:"ai.instruction" validate:"required,min=1"`
	AITimeout     time.Duration `koanf:"ai.timeout" validate:"required,min=1s,max=10m"`

	// Telegram Settings
	TelegramToken          string `koanf:"telegram.token" validate:"required"`
	TelegramAdminID        int64  `koanf:"telegram.admin_id" validate:"required,gt=0"`
	TelegramWelcomeMessage string `koanf:"telegram.messages.welcome" validate:"required"`
	TelegramNotAuthMessage string `koanf:"telegram.messages.not_authorized" validate:"required"`
	TelegramProvideMessage string `koanf:"telegram.messages.provide_message" validate:"required"`
	TelegramAIErrorMessage string `koanf:"telegram.messages.ai_error" validate:"required"`
	TelegramGeneralError   string `koanf:"telegram.messages.general_error" validate:"required"`
	TelegramHistoryReset   string `koanf:"telegram.messages.history_reset" validate:"required"`

	// Logging Settings
	LogLevel  string `koanf:"log.level" validate:"required,oneof=debug info warn error"`
	LogFormat string `koanf:"log.format" validate:"required,oneof=json text"`
}

var defaults = map[string]interface{}{
	"ai.base_url":                       "https://api.openai.com/v1",
	"ai.model":                          "gpt-4o",
	"ai.temperature":                    1.0,
	"ai.instruction":                    "You are a helpful assistant focused on providing clear and accurate responses.",
	"ai.timeout":                        time.Duration(2 * time.Minute),
	"telegram.messages.welcome":         "👋 Welcome! I'm ready to assist you. Use /mrl followed by your message to start a conversation.",
	"telegram.messages.not_authorized":  "🚫 Access denied. Please contact the administrator.",
	"telegram.messages.provide_message": "ℹ️ Please provide a message with your command.",
	"telegram.messages.ai_error":        "🤖 Unable to process request. Please try again.",
	"telegram.messages.general_error":   "❌ An error occurred. Please try again later.",
	"telegram.messages.history_reset":   "🔄 Chat history has been cleared.",
	"log.level":                         "info",
	"log.format":                        "json",
}

func LoadConfig() (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("error loading defaults: %w", err)
	}

	configFileLoaded := false
	if err := k.Load(file.Provider("config.yaml"), yaml.Parser()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		slog.Info("config file not found, using defaults and environment")
	} else {
		configFileLoaded = true
	}

	if err := k.Load(env.Provider("BOT", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "BOT_")), "_", ".", -1)
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}

	var config Config
	if err := k.UnmarshalWithConf("", &config, koanf.UnmarshalConf{Tag: "koanf", FlatPaths: true}); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	v := validator.New()
	if err := v.Struct(config); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var msg string
			for i, e := range validationErrors {
				if i > 0 {
					msg += ", "
				}
				msg += e.Field() + ": " + e.Tag()
			}
			return nil, fmt.Errorf("validation errors: %s", msg)
		}
		return nil, fmt.Errorf("validation error: %w", err)
	}

	slog.Info("configuration loaded", "config_file", configFileLoaded)

	return &config, nil
}
