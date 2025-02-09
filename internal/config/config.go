package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds the application configuration variables.
type Config struct {
	TelegramToken       string
	TelegramAdminUID    int64
	TelegramUserTimeout float64
	OpenAIToken         string
	OpenAIInstruction   string
	OpenAIModel         string
	OpenAITemperature   float32
	OpenAITopP          float32
	DBName              string
}

// LoadConfig reads configuration from environment variables using viper.
func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("murailobot")
	viper.AutomaticEnv()

	viper.SetDefault("telegram_user_timeout", 5.0)
	viper.SetDefault("openai_model", "gpt-4o")
	viper.SetDefault("openai_temperature", 0.5)
	viper.SetDefault("openai_top_p", 0.5)
	viper.SetDefault("db_name", "storage.db")

	required := []string{"telegram_token", "telegram_admin_uid", "openai_token", "openai_instruction"}
	for _, key := range required {
		if !viper.IsSet(key) {
			return nil, fmt.Errorf("missing required env var: %s", key)
		}
	}

	cfg := &Config{
		TelegramToken:       viper.GetString("telegram_token"),
		TelegramAdminUID:    viper.GetInt64("telegram_admin_uid"),
		TelegramUserTimeout: viper.GetFloat64("telegram_user_timeout"),
		OpenAIToken:         viper.GetString("openai_token"),
		OpenAIInstruction:   viper.GetString("openai_instruction"),
		OpenAIModel:         viper.GetString("openai_model"),
		OpenAITemperature:   float32(viper.GetFloat64("openai_temperature")),
		OpenAITopP:          float32(viper.GetFloat64("openai_top_p")),
		DBName:              viper.GetString("db_name"),
	}

	if cfg.TelegramUserTimeout <= 0 {
		return nil, fmt.Errorf("telegram_user_timeout must be > 0")
	}
	return cfg, nil
}
