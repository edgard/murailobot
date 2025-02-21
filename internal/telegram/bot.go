package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/db"
)

func (b *Bot) sendContinuousTyping(ctx context.Context, bot *gotgbot.Bot, chatID int64) {
	b.typing.sendContinuousTyping(ctx, bot, chatID)
}

type commandHandler struct {
	bot *Bot
}

func (h *commandHandler) Name() string {
	return "CommandHandler"
}

func (h *commandHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	msg := ctx.EffectiveMessage
	if msg == nil || msg.Text == "" {
		return false
	}
	return strings.HasPrefix(msg.Text, "/")
}

func (h *commandHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return fmt.Errorf("%w: message is nil", ErrBot)
	}
	if msg.Text == "" {
		return nil
	}

	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return nil
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.TrimPrefix(parts[0], "/")
	cmd = strings.Split(cmd, "@")[0] // Strip bot username from command

	switch cmd {
	case "start":
		return h.bot.handleStart(b, ctx)
	case "mrl":
		return h.bot.handleChatMessage(b, ctx)
	case "mrl_reset":
		return h.bot.handleChatHistoryReset(b, ctx)
	}
	return nil
}

// New creates a new Telegram bot instance
func New(cfg *Config, security *SecurityConfig, database db.Database, ai *ai.AI) (*Bot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: config is nil", ErrBot)
	}
	if security == nil {
		return nil, fmt.Errorf("%w: security config is nil", ErrBot)
	}
	if database == nil {
		return nil, fmt.Errorf("%w: database is nil", ErrBot)
	}
	if ai == nil {
		return nil, fmt.Errorf("%w: AI client is nil", ErrBot)
	}

	bot, err := gotgbot.NewBot(cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create bot: %v", ErrBot, err)
	}

	// Set adaptive concurrent update handling based on system resources
	maxRoutines := cfg.Polling.MaxRoutines
	if maxRoutines <= 0 {
		// Default to number of CPU cores + 2 for I/O bound operations
		maxRoutines = runtime.NumCPU() + 2
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			if err.Error() == "dispatcher: max routines reached" {
				slog.Warn("max concurrent updates reached, will retry",
					"max_routines", maxRoutines,
					"update_id", ctx.Update.UpdateId,
					"active_routines", maxRoutines,
				)
				// Return noop to let Telegram's built-in retry mechanism handle it
				return ext.DispatcherActionNoop
			}
			slog.Error("error handling update",
				"error", err,
				"update_id", ctx.Update.UpdateId,
			)
			return ext.DispatcherActionNoop
		},
		MaxRoutines: maxRoutines,
	})

	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{
		ErrorLog: nil,
	})

	b := &Bot{
		Bot:      bot,
		updater:  updater,
		db:       database,
		ai:       ai,
		cfg:      cfg,
		security: security,
		typing:   newTypingManager(cfg),
	}

	// Register command handler
	dispatcher.AddHandlerToGroup(&commandHandler{bot: b}, 0)

	return b, nil
}

func (b *Bot) Start(ctx context.Context) error {
	if err := b.updater.StartPolling(b.Bot, &ext.PollingOpts{
		DropPendingUpdates: b.cfg.Polling.DropPendingUpdates,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: int64(b.cfg.Polling.Timeout.Seconds()),
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: b.cfg.Polling.RequestTimeout,
			},
		},
	}); err != nil {
		return fmt.Errorf("%w: failed to start polling: %v", ErrBot, err)
	}

	slog.Info("bot started")

	<-ctx.Done()
	slog.Info("shutting down bot")

	if err := b.updater.Stop(); err != nil { // Stop accepting new updates
		return fmt.Errorf("%w: failed to stop updater: %v", ErrBot, err)
	}

	return nil
}
