package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load loads and validates configuration from:
// 1. Default values
// 2. config.yaml file
// 3. BOT_* environment variables
func Load() (*Config, error) {
	// Set defaults first
	setDefaults()

	// Create initial config with defaults
	cfg := &Config{
		Log: LogConfig{
			Level:  DefaultLogLevel,
			Format: DefaultLogFormat,
		},
		OpenAI: OpenAIConfig{
			BaseURL:     DefaultOpenAIBaseURL,
			Model:       DefaultOpenAIModel,
			Temperature: DefaultOpenAITemperature,
			TopP:        DefaultOpenAITopP,
			Timeout:     DefaultOpenAITimeout,
			Instruction: DefaultOpenAIInstruction,
		},
		Bot: BotConfig{
			MaxMessageLength:    DefaultBotMaxMessageLength,
			TypingInterval:      DefaultBotTypingInterval,
			PollTimeout:         DefaultBotPollTimeout,
			RequestTimeout:      DefaultBotRequestTimeout,
			MaxRoutines:         DefaultBotMaxRoutines,
			DropPendingUpdates:  DefaultBotDropPendingUpdates,
			TypingActionTimeout: DefaultBotTypingActionTimeout,
			DBOperationTimeout:  DefaultBotDBOperationTimeout,
			AIRequestTimeout:    DefaultBotAIRequestTimeout,
			Messages:            DefaultBotMessages,
			Commands:            DefaultBotCommands,
		},
		Database: DatabaseConfig{
			Name:            DefaultDBName,
			MaxOpenConns:    DefaultDBMaxOpenConns,
			MaxIdleConns:    DefaultDBMaxIdleConns,
			ConnMaxLifetime: DefaultDBConnMaxLifetime,
			MaxMessageSize:  DefaultDBMaxMessageSize,
		},
	}

	// Try to load config file (optional)
	if err := loadConfig(); err != nil {
		return nil, fmt.Errorf("%w: failed to load config file: %v", ErrConfiguration, err)
	}

	// Unmarshal config file over defaults
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("%w: failed to parse config: %v", ErrConfiguration, err)
	}

	// Validate the complete config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfiguration, err)
	}

	return cfg, nil
}

// loadConfig initializes and loads the configuration using viper
func loadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Setup environment variables
	viper.SetEnvPrefix("BOT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Allow missing config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %v", err)
		}
		// Config file not found is okay, we'll use defaults
	}

	return nil
}

// setDefaults sets default values for optional configuration parameters
func setDefaults() {
	// Log defaults
	viper.SetDefault("log.level", DefaultLogLevel)
	viper.SetDefault("log.format", DefaultLogFormat)

	// Database defaults
	viper.SetDefault("database.name", DefaultDBName)
	viper.SetDefault("database.max_open_conns", DefaultDBMaxOpenConns)
	viper.SetDefault("database.max_idle_conns", DefaultDBMaxIdleConns)
	viper.SetDefault("database.conn_max_lifetime", DefaultDBConnMaxLifetime)
	viper.SetDefault("database.max_message_size", DefaultDBMaxMessageSize)

	// OpenAI defaults
	viper.SetDefault("openai.model", DefaultOpenAIModel)
	viper.SetDefault("openai.temperature", DefaultOpenAITemperature)
	viper.SetDefault("openai.top_p", DefaultOpenAITopP)
	viper.SetDefault("openai.timeout", DefaultOpenAITimeout)
	viper.SetDefault("openai.base_url", DefaultOpenAIBaseURL)
	viper.SetDefault("openai.instruction", DefaultOpenAIInstruction)

	// Bot defaults
	viper.SetDefault("bot.max_message_length", DefaultBotMaxMessageLength)
	viper.SetDefault("bot.typing_interval", DefaultBotTypingInterval)
	viper.SetDefault("bot.poll_timeout", DefaultBotPollTimeout)
	viper.SetDefault("bot.request_timeout", DefaultBotRequestTimeout)
	viper.SetDefault("bot.max_routines", DefaultBotMaxRoutines)
	viper.SetDefault("bot.drop_pending_updates", DefaultBotDropPendingUpdates)

	// Bot operation timeouts
	viper.SetDefault("bot.typing_action_timeout", DefaultBotTypingActionTimeout)
	viper.SetDefault("bot.db_operation_timeout", DefaultBotDBOperationTimeout)
	viper.SetDefault("bot.ai_request_timeout", DefaultBotAIRequestTimeout)

	// Bot messages defaults
	viper.SetDefault("bot.messages.welcome", DefaultBotMessages.Welcome)
	viper.SetDefault("bot.messages.not_authorized", DefaultBotMessages.NotAuthorized)
	viper.SetDefault("bot.messages.history_reset", DefaultBotMessages.HistoryReset)
	viper.SetDefault("bot.messages.provide_message", DefaultBotMessages.ProvideMessage)
	viper.SetDefault("bot.messages.general_error", DefaultBotMessages.GeneralError)
	viper.SetDefault("bot.messages.ai_error", DefaultBotMessages.AIError)
	viper.SetDefault("bot.messages.message_too_long", DefaultBotMessages.MessageTooLong)

	// Bot commands defaults
	viper.SetDefault("bot.commands", DefaultBotCommands)
}
