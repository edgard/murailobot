package telegram

import (
	"time"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/scheduler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Default bot operational parameters.
const (
	// defaultUpdateOffset defines the starting point for update retrieval (0 = from latest).
	defaultUpdateOffset = 0

	// defaultUpdateTimeout defines how long to wait for updates (in seconds).
	defaultUpdateTimeout = 60

	// defaultTypingInterval defines how often to send typing indicators to show the bot is active.
	defaultTypingInterval = 5 * time.Second
)

// commands defines bot command descriptions for Telegram.
type commands struct {
	Start    string
	Reset    string
	Analyze  string
	Profiles string
	EditUser string
}

// messages defines bot response templates.
type messages struct {
	Welcome        string
	Unauthorized   string
	Provide        string
	GeneralError   string
	HistoryReset   string
	Analyzing      string
	NoProfiles     string
	InvalidUserID  string
	InvalidField   string
	UpdateSuccess  string
	UserEditUsage  string
	ProfilesHeader string
}

// botConfig holds bot authentication and response configuration.
type botConfig struct {
	Token    string
	AdminID  int64
	Commands commands
	Messages messages
}

// Bot implements a Telegram bot with AI capabilities.
type Bot struct {
	api       *tgbotapi.BotAPI
	db        db.Database
	ai        ai.Service
	scheduler scheduler.Scheduler
	cfg       *botConfig
	running   chan struct{}
}
