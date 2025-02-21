package telegram

import (
	"errors"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
)

var (
	ErrBot          = errors.New("bot error")
	ErrUnauthorized = errors.New("unauthorized access")
)

// Config holds Telegram-specific configuration
type Config struct {
	Token          string        `mapstructure:"token"`
	AdminUID       int64         `mapstructure:"admin_uid"`
	Messages       BotMessages   `mapstructure:"messages"`
	Polling        PollingConfig `mapstructure:"polling"`
	TypingInterval time.Duration `mapstructure:"typing_interval"`

	// Operation timeouts
	TypingActionTimeout time.Duration `mapstructure:"typing_action_timeout"`
	DBOperationTimeout  time.Duration `mapstructure:"db_operation_timeout"`
	AIRequestTimeout    time.Duration `mapstructure:"ai_request_timeout"`
}

// BotMessages holds message templates
type BotMessages struct {
	Welcome        string `mapstructure:"welcome"`
	NotAuthorized  string `mapstructure:"not_authorized"`
	ProvideMessage string `mapstructure:"provide_message"`
	MessageTooLong string `mapstructure:"message_too_long"`
	AIError        string `mapstructure:"ai_error"`
	GeneralError   string `mapstructure:"general_error"`
	HistoryReset   string `mapstructure:"history_reset"`
}

// PollingConfig holds polling-related settings
type PollingConfig struct {
	Timeout            time.Duration `mapstructure:"timeout"`
	RequestTimeout     time.Duration `mapstructure:"request_timeout"`
	MaxRoutines        int           `mapstructure:"max_routines"`
	DropPendingUpdates bool          `mapstructure:"drop_pending_updates"`
}

// SecurityConfig holds security-related settings
type SecurityConfig struct {
	MaxMessageLength int     `mapstructure:"max_message_length"`
	AllowedUserIDs   []int64 `mapstructure:"allowed_user_ids"`
	BlockedUserIDs   []int64 `mapstructure:"blocked_user_ids"`
}

// Bot represents a Telegram bot instance
type Bot struct {
	*gotgbot.Bot
	updater  *ext.Updater
	db       db.Database
	ai       *ai.AI
	cfg      *Config
	security *SecurityConfig
	typing   *typingManager
}
