package config

import (
	"errors"
	"os"
	"strings"

	errs "github.com/edgard/murailobot/internal/errors"
	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// LoadConfig loads configuration from defaults, file, and environment variables.
// Environment variables are prefixed with BOT_ and use underscore as separator.
// Example: BOT_AI_TOKEN -> ai.token.
func LoadConfig() (*Config, error) {
	if err := loadDefaults(); err != nil {
		return nil, err
	}

	konfig := koanf.New(".")

	// Load default configuration values
	if err := konfig.Load(confmap.Provider(defaultConfig, "."), nil); err != nil {
		return nil, errs.NewConfigError("failed to load default configuration", err)
	}

	// Load configuration from file if it exists
	if err := konfig.Load(file.Provider("config.yaml"), yaml.Parser()); err != nil {
		if !os.IsNotExist(err) {
			return nil, errs.NewConfigError("failed to load configuration file", err)
		}
		// Continue with defaults if file doesn't exist
	}

	// Load configuration from environment variables
	envProvider := env.Provider("BOT", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "BOT_")),
			"_",
			".",
		)
	})
	if err := konfig.Load(envProvider, nil); err != nil {
		// Add context about which environment variables might be problematic
		vars := os.Environ()

		var botVars []string

		for _, v := range vars {
			if strings.HasPrefix(v, "BOT_") {
				botVars = append(botVars, strings.Split(v, "=")[0])
			}
		}

		msg := "failed to load environment variables"
		if len(botVars) > 0 {
			msg = msg + " (found BOT_ vars: " + strings.Join(botVars, ", ") + ")"
		}

		return nil, errs.NewConfigError(msg, err)
	}

	// Parse configuration into struct
	var config Config
	if err := konfig.UnmarshalWithConf("", &config, koanf.UnmarshalConf{
		Tag:       "koanf",
		FlatPaths: true,
	}); err != nil {
		return nil, errs.NewConfigError("failed to parse configuration", err)
	}

	// Validate configuration values
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateConfig performs validation of the configuration using struct tags
// and returns an error with all validation failures.
func validateConfig(config *Config) error {
	if config == nil {
		return errs.NewValidationError("nil config", nil)
	}

	// First validate required fields and basic types
	v := validator.New()
	if err := v.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			var validationMsgs []string
			for _, e := range validationErrors {
				validationMsgs = append(validationMsgs,
					formatValidationError(e.Field(), e.Tag(), e.Param()))
			}

			if len(validationMsgs) > 0 {
				return errs.NewValidationError(
					strings.Join(validationMsgs, "; "),
					nil,
				)
			}
		}

		return errs.NewValidationError("validation failed", err)
	}

	// Additional validation for specific fields
	if err := validateAIConfig(config); err != nil {
		return err
	}

	if err := validateTelegramConfig(config); err != nil {
		return err
	}

	return nil
}

// validateAIConfig performs additional validation on AI-specific configuration.
func validateAIConfig(config *Config) error {
	if config.AITimeout <= 0 {
		return errs.NewValidationError("AI timeout must be positive", nil)
	}

	if config.AITemperature < 0 || config.AITemperature > 2 {
		return errs.NewValidationError("AI temperature must be between 0 and 2", nil)
	}

	if config.AIInstruction == "" {
		return errs.NewValidationError("AI instruction cannot be empty", nil)
	}

	return nil
}

// validateTelegramConfig performs additional validation on Telegram-specific configuration.
func validateTelegramConfig(config *Config) error {
	if config.TelegramAdminID <= 0 {
		return errs.NewValidationError("Telegram admin ID must be positive", nil)
	}

	if config.TelegramWelcomeMessage == "" {
		return errs.NewValidationError("Telegram welcome message cannot be empty", nil)
	}

	return nil
}

// formatValidationError creates a human-readable validation error message.
func formatValidationError(field, tag, param string) string {
	switch tag {
	case "required":
		return field + " is required"
	case "min":
		return field + " must be at least " + param
	case "max":
		return field + " must be at most " + param
	case "oneof":
		return field + " must be one of: " + param
	default:
		return field + " failed " + tag + " validation"
	}
}

// loadDefaults ensures required default configuration is available.
func loadDefaults() error {
	if defaultConfig == nil {
		return errs.NewConfigError("default configuration not initialized", nil)
	}

	return nil
}
