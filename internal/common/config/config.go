// Package config provides configuration handling for MurailoBot.
package config

import (
	"fmt"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

// Config contains all the application configuration values.
type Config struct {
	// Bot configuration
	BotToken             string `koanf:"bot_token"        validate:"required"`
	BotAdminID           int64  `koanf:"bot_admin_id"     validate:"required"`
	BotMsgWelcome        string `koanf:"bot_msg_welcome"`
	BotMsgNotAuthorized  string `koanf:"bot_msg_not_authorized"`
	BotMsgProvideMessage string `koanf:"bot_msg_provide_message"`
	BotMsgGeneralError   string `koanf:"bot_msg_general_error"`
	BotMsgHistoryReset   string `koanf:"bot_msg_history_reset"`
	BotMsgAnalyzing      string `koanf:"bot_msg_analyzing"`
	BotMsgNoProfiles     string `koanf:"bot_msg_no_profiles"`
	BotMsgProfilesHeader string `koanf:"bot_msg_profiles_header"`

	BotCmdStart    string `koanf:"bot_cmd_start"`
	BotCmdReset    string `koanf:"bot_cmd_reset"`
	BotCmdAnalyze  string `koanf:"bot_cmd_analyze"`
	BotCmdProfiles string `koanf:"bot_cmd_profiles"`
	BotCmdEditUser string `koanf:"bot_cmd_edit_user"`

	// AI configuration
	AIToken              string        `koanf:"ai_token"               validate:"required"`
	AIBaseURL            string        `koanf:"ai_base_url"`
	AIModel              string        `koanf:"ai_model"`
	AITemperature        float32       `koanf:"ai_temperature"`
	AIMaxContextTokens   int           `koanf:"ai_max_context_tokens"`
	AITimeout            time.Duration `koanf:"ai_timeout"`
	AIInstruction        string        `koanf:"ai_instruction"`
	AIProfileInstruction string        `koanf:"ai_profile_instruction"`

	// Database configuration
	DBPath string `koanf:"db_path"`

	// Logging configuration
	LogFormat string `koanf:"log_format"`
	LogLevel  string `koanf:"log_level"`
}

// Load reads configuration from a YAML file and validates it.
func Load(filePath string, logger *zap.Logger) (*Config, error) {
	k := koanf.New(".")

	// Load configuration
	if err := k.Load(file.Provider(filePath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", filePath, err)
	}

	// Apply defaults for optional values
	applyDefaults(k, logger)

	// Unmarshal to struct
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	logger.Info("configuration loaded successfully", zap.String("config_file", filePath))

	return &cfg, nil
}

func applyDefaults(k *koanf.Koanf, logger *zap.Logger) {
	// Bot default messages
	if !k.Exists("bot_msg_welcome") {
		if err := k.Set("bot_msg_welcome", "Hello! I'm MurailoBot. Mention me in a group message to chat!"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_welcome"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_not_authorized") {
		if err := k.Set("bot_msg_not_authorized", "You are not authorized to use this command."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_not_authorized"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_provide_message") {
		if err := k.Set("bot_msg_provide_message", "Please provide a message."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_provide_message"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_general_error") {
		if err := k.Set("bot_msg_general_error", "An error occurred. Please try again later."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_general_error"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_history_reset") {
		if err := k.Set("bot_msg_history_reset", "Message history and user profiles have been reset."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_history_reset"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_analyzing") {
		if err := k.Set("bot_msg_analyzing", "Analyzing messages and updating user profiles..."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_analyzing"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_no_profiles") {
		if err := k.Set("bot_msg_no_profiles", "No user profiles found. Try analyzing more messages first."); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_no_profiles"), zap.Error(err))
		}
	}

	if !k.Exists("bot_msg_profiles_header") {
		if err := k.Set("bot_msg_profiles_header", "📊 User Profiles:\n\n"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_msg_profiles_header"), zap.Error(err))
		}
	}

	// Bot command descriptions
	if !k.Exists("bot_cmd_start") {
		if err := k.Set("bot_cmd_start", "Start the bot and get a welcome message"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_cmd_start"), zap.Error(err))
		}
	}

	if !k.Exists("bot_cmd_reset") {
		if err := k.Set("bot_cmd_reset", "Reset message history and profiles"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_cmd_reset"), zap.Error(err))
		}
	}

	if !k.Exists("bot_cmd_analyze") {
		if err := k.Set("bot_cmd_analyze", "Analyze messages and update profiles"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_cmd_analyze"), zap.Error(err))
		}
	}

	if !k.Exists("bot_cmd_profiles") {
		if err := k.Set("bot_cmd_profiles", "Show user profiles"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_cmd_profiles"), zap.Error(err))
		}
	}

	if !k.Exists("bot_cmd_edit_user") {
		if err := k.Set("bot_cmd_edit_user", "Edit a user profile field"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "bot_cmd_edit_user"), zap.Error(err))
		}
	}

	// AI configuration defaults
	if !k.Exists("ai_base_url") {
		if err := k.Set("ai_base_url", "https://api.openai.com/v1"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_base_url"), zap.Error(err))
		}
	}

	if !k.Exists("ai_model") {
		if err := k.Set("ai_model", "gpt-3.5-turbo"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_model"), zap.Error(err))
		}
	}

	if !k.Exists("ai_temperature") {
		if err := k.Set("ai_temperature", 0.8); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_temperature"), zap.Error(err))
		}
	}

	if !k.Exists("ai_timeout") {
		if err := k.Set("ai_timeout", 30*time.Second); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_timeout"), zap.Error(err))
		}
	}

	if !k.Exists("ai_max_context_tokens") {
		if err := k.Set("ai_max_context_tokens", 4000); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_max_context_tokens"), zap.Error(err))
		}
	}

	// Database configuration defaults
	if !k.Exists("db_path") {
		if err := k.Set("db_path", "storage.db"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "db_path"), zap.Error(err))
		}
	}

	// Logging configuration defaults
	if !k.Exists("log_format") {
		if err := k.Set("log_format", "json"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "log_format"), zap.Error(err))
		}
	}

	if !k.Exists("log_level") {
		if err := k.Set("log_level", "info"); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "log_level"), zap.Error(err))
		}
	}

	// Default system prompt if not provided
	if !k.Exists("ai_instruction") {
		if err := k.Set("ai_instruction", `You are a helpful and friendly assistant in a Telegram group chat.
Your responses should be concise, helpful, and conversational.

When asked about technical topics, provide accurate information with examples where appropriate.
For subjective questions, present multiple perspectives and avoid strong bias.
Keep responses concise and to the point, use markdown formatting sparingly for readability.
Be friendly and casual in tone while remaining respectful.`); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_instruction"), zap.Error(err))
		}
	}

	// Default profile generation prompt if not provided
	if !k.Exists("ai_profile_instruction") {
		if err := k.Set("ai_profile_instruction", `You are a behavioral analyst with expertise in psychology, linguistics, and social dynamics.

Your task is to analyze chat messages and build concise, meaningful profiles of users. Pay special attention to:

1. Names and nicknames they go by (displayNames)
2. Mentions of where they're from (originLocation)
3. Where they currently live (currentLocation)
4. Approximate age range based on references or communication style (ageRange)
5. Personality traits, interests, and characteristics (traits)

Focus on what the text directly reveals. Make cautious, conservative inferences where evidence is limited.
If information is missing, leave those fields empty rather than guessing.`); err != nil {
			logger.Error("failed to set default config value", zap.String("key", "ai_profile_instruction"), zap.Error(err))
		}
	}
}
