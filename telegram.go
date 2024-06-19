package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"go.uber.org/zap"
)

// Telegram encapsulates the bot's logic and dependencies.
type Telegram struct {
	bot     *gotgbot.Bot
	updater *ext.Updater
	db      *DB
	oai     *OpenAI
	config  *Config
	logger  *zap.Logger
}

// NewTelegram creates a new Telegram bot instance.
func NewTelegram(token string, db *DB, oai *OpenAI, config *Config, logger *zap.Logger) (*Telegram, error) {
	bot, err := gotgbot.NewBot(token, nil)
	if err != nil {
		return nil, err
	}

	tg := &Telegram{
		bot:    bot,
		db:     db,
		oai:    oai,
		config: config,
		logger: logger,
	}
	tg.updater = ext.NewUpdater(tg.setupDispatcher(), nil)
	return tg, nil
}

// Start starts the Telegram bot.
func (tg *Telegram) Start() {
	err := tg.updater.StartPolling(tg.bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		tg.logger.Fatal("Error starting polling", zap.Error(err))
	}

	tg.logger.Info("Started Telegram Bot", zap.String("username", tg.bot.User.Username))
	tg.updater.Idle()
}

func (tg *Telegram) setupDispatcher() *ext.Dispatcher {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			tg.logger.Warn("Error occurred while handling update", zap.Error(err))
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	dispatcher.AddHandler(handlers.NewCommand("start", tg.handleStartRequest))
	dispatcher.AddHandler(handlers.NewCommand("piu", tg.handlePiuRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl", tg.handleMrlRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl_reset", tg.handleMrlResetRequest))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, tg.handleIncomingMessage))
	return dispatcher
}

func (tg *Telegram) handleIncomingMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage.ForwardOrigin == nil {
		tg.logger.Info("Received non-forward message, ignoring", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))
		return nil
	}

	tg.logger.Info("Received forward message", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	msgRef := MessageRef{MessageID: ctx.EffectiveMessage.MessageId, ChatID: ctx.EffectiveMessage.Chat.Id, LastUsed: time.Now()}
	err := tg.db.AddMessageRef(&msgRef)
	if err != nil {
		return err
	}

	return tg.sendTelegramMessage(ctx, "Mensagem adicionada ao banco de dados!")
}

func (tg *Telegram) handleStartRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	return tg.sendTelegramMessage(ctx, "Olá! Me encaminhe uma mensagem para guardar.")
}

func (tg *Telegram) handlePiuRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	tg.logger.Info("Received PIU request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	user, err := tg.db.GetOrCreateUser(ctx.EffectiveMessage.From.Id, tg.config.TelegramUserTimeout)
	if err != nil {
		return err
	}

	if time.Since(user.LastUsed).Minutes() <= tg.config.TelegramUserTimeout {
		tg.logger.Info("User on timeout", zap.Int64("user_id", user.UserID), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Time("last_used", user.LastUsed))
		return nil
	}

	if err := tg.db.UpdateUserLastUsed(user); err != nil {
		return err
	}

	msgRef, err := tg.db.GetRandomMessageRef()
	if err != nil {
		return err
	}

	return tg.forwardTelegramMessage(ctx, msgRef.ChatID, msgRef.MessageID)
}

func (tg *Telegram) handleMrlRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	tg.logger.Info("Received MRL request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}

	message := strings.TrimSpace(strings.TrimPrefix(ctx.EffectiveMessage.Text, "/mrl"))

	gptHistory, err := tg.db.GetRecentChatHistory(30)
	if err != nil {
		return err
	}

	messages := []map[string]string{{"role": "system", "content": tg.config.OpenAIInstruction}}

	sort.Slice(gptHistory, func(i, j int) bool {
		return gptHistory[i].LastUsed.Before(gptHistory[j].LastUsed)
	})

	for _, history := range gptHistory {
		userName := history.UserName
		if userName == "" {
			userName = "Unknown User"
		}
		messages = append(messages, map[string]string{
			"role": "user", "content": fmt.Sprintf("[UID: %d] %s [%s]: %s", history.UserID, userName, history.LastUsed.Format(time.RFC3339), history.UserMsg),
		})
		messages = append(messages, map[string]string{
			"role": "assistant", "content": history.BotMsg,
		})
	}

	userName := ctx.EffectiveMessage.From.Username
	if userName == "" {
		userName = "Unknown User"
	}
	messages = append(messages, map[string]string{
		"role": "user", "content": fmt.Sprintf("[UID: %d] %s [%s]: %s", ctx.EffectiveMessage.From.Id, userName, time.Now().Format(time.RFC3339), message),
	})

	content, err := tg.oai.Call(messages, 1)
	if err != nil {
		return err
	}

	if err := tg.sendTelegramMessage(ctx, content); err != nil {
		return err
	}

	historyRecord := ChatHistory{UserID: ctx.EffectiveMessage.From.Id, UserName: ctx.EffectiveMessage.From.Username, UserMsg: message, BotMsg: content, LastUsed: time.Now()}
	if err := tg.db.AddChatHistory(&historyRecord); err != nil {
		return err
	}

	return nil
}

func (tg *Telegram) handleMrlResetRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	tg.logger.Info("Received MRL_RESET request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}

	if ctx.EffectiveMessage.From.Id != tg.config.TelegramAdminUID {
		ctx.EffectiveMessage.Reply(b, "You are not authorized to use this command.", nil)
		return nil
	}

	if err := tg.db.ClearChatHistory(); err != nil {
		return err
	}

	_, err := ctx.EffectiveMessage.Reply(b, "History has been reset.", nil)
	return err
}

func (tg *Telegram) sendTelegramMessage(ctx *ext.Context, text string) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := ctx.EffectiveMessage.Reply(tg.bot, text, nil)
	if err != nil {
		return err
	}
	return nil
}

func (tg *Telegram) forwardTelegramMessage(ctx *ext.Context, forwardChatID int64, forwardMessageID int64) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := tg.bot.ForwardMessage(ctx.EffectiveChat.Id, forwardChatID, forwardMessageID, nil)
	if err != nil {
		return err
	}
	return nil
}
