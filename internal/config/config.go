package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// BOT_AI_TOKEN -> ai.token.
func LoadConfig() (*Config, error) {
	konfig := koanf.New(".")

	// Load default configuration values
	if err := konfig.Load(confmap.Provider(defaultConfig, "."), nil); err != nil {
		return nil, fmt.Errorf("error loading defaults: %w", err)
	}

	// Load configuration from file if it exists
	if err := konfig.Load(file.Provider("config.yaml"), yaml.Parser()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
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
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}

	// Parse configuration into struct
	var config Config
	if err := konfig.UnmarshalWithConf("", &config, koanf.UnmarshalConf{
		Tag:       "koanf",
		FlatPaths: true,
	}); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
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
	v := validator.New()
	if err := v.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			var validationMsgs []string
			for _, e := range validationErrors {
				validationMsgs = append(validationMsgs, e.Field()+": "+e.Tag())
			}

			if len(validationMsgs) > 0 {
				return fmt.Errorf("%w: %s", ErrValidation, strings.Join(validationMsgs, ", "))
			}
		}

		return fmt.Errorf("validation error: %w", err)
	}

	return nil
}
