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

// LoadConfig loads and validates configuration from multiple sources in the following order:
//  1. Default values (lowest priority)
//  2. Configuration file (config.yaml)
//  3. Environment variables (highest priority)
//
// Environment variables should be prefixed with BOT_ and use underscore as separator.
// For example, BOT_OPENAI_TOKEN will set the OpenAIToken field.
//
// Example usage:
//
//	cfg, err := config.LoadConfig()
//	if err != nil {
//	    log.Fatal(err)
//	}
func LoadConfig() (*Config, error) {
	k := koanf.New(".")

	// Load defaults
	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("error loading defaults: %w", err)
	}

	// Load config file if exists
	if err := k.Load(file.Provider("config.yaml"), yaml.Parser()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Load environment variables
	if err := k.Load(env.Provider("BOT", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "BOT_")), "_", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}

	var config Config
	if err := k.UnmarshalWithConf("", &config, koanf.UnmarshalConf{
		Tag:       "koanf",
		FlatPaths: true,
	}); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateConfig performs validation of the config struct using the validator tags.
// It returns a formatted error message containing all validation failures when validation fails.
// For example: "validation error: OpenAIToken: required, OpenAIModel: required".
func validateConfig(config *Config) error {
	v := validator.New()
	if err := v.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			var msgs []string
			for _, e := range validationErrors {
				msgs = append(msgs, e.Field()+": "+e.Tag())
			}

			if len(msgs) > 0 {
				return fmt.Errorf("%w: %s", ErrValidation, strings.Join(msgs, ", "))
			}
		}

		return fmt.Errorf("validation error: %w", err)
	}

	return nil
}
