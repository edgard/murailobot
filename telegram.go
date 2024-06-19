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

// TelegramBot encapsulates the bot's logic and dependencies.
type TelegramBot struct {
	bot     *gotgbot.Bot
	updater *ext.Updater
}

// Init initializes the Telegram bot.
func (tb *TelegramBot) Init() error {
	var err error
	tb.bot, err = gotgbot.NewBot(config.TelegramToken, nil)
	if err != nil {
		return err
	}
	return nil
}

// Start starts the Telegram bot.
func (tb *TelegramBot) Start() {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			logger.Warn("Error occurred while handling update", zap.Error(err))
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	tb.updater = ext.NewUpdater(dispatcher, nil)

	dispatcher.AddHandler(handlers.NewCommand("start", tb.handleStartRequest))
	dispatcher.AddHandler(handlers.NewCommand("piu", tb.handlePiuRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl", tb.handleMrlRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl_reset", tb.handleMrlResetRequest))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, tb.handleIncomingMessage))

	err := tb.updater.StartPolling(tb.bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		logger.Fatal("Error starting polling", zap.Error(err))
	}

	logger.Info("Started Telegram Bot", zap.String("username", tb.bot.User.Username))
	tb.updater.Idle()
}

func (tb *TelegramBot) handleIncomingMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage.ForwardOrigin == nil {
		logger.Info("Received non-forward message, ignoring", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))
		return nil
	}

	logger.Info("Received forward message", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	msgRef := MessageRef{MessageID: ctx.EffectiveMessage.MessageId, ChatID: ctx.EffectiveMessage.Chat.Id, LastUsed: time.Now()}
	err := db.AddMessageRef(&msgRef)
	if err != nil {
		return err
	}

	return tb.sendTelegramMessage(ctx, "Mensagem adicionada ao banco de dados!")
}

func (tb *TelegramBot) handleStartRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	return tb.sendTelegramMessage(ctx, "Olá! Me encaminhe uma mensagem para guardar.")
}

func (tb *TelegramBot) handlePiuRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	logger.Info("Received PIU request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	user, err := db.GetOrCreateUser(ctx)
	if err != nil {
		return err
	}

	if time.Since(user.LastUsed).Minutes() <= config.TelegramUserTimeout {
		logger.Info("User on timeout", zap.Int64("user_id", user.UserID), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Time("last_used", user.LastUsed))
		return nil
	}

	if err := db.UpdateUserLastUsed(user); err != nil {
		return err
	}

	msgRef, err := db.GetRandomMessageRef()
	if err != nil {
		return err
	}

	return tb.forwardTelegramMessage(ctx, msgRef.ChatID, msgRef.MessageID)
}

func (tb *TelegramBot) handleMrlRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	logger.Info("Received MRL request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}

	message := strings.TrimSpace(strings.TrimPrefix(ctx.EffectiveMessage.Text, "/mrl"))

	gptHistory, err := db.GetRecentChatHistory(30)
	if err != nil {
		return err
	}

	messages := []map[string]string{{"role": "system", "content": config.OpenAIInstruction}}

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

	content, err := oai.Call(messages, 1)
	if err != nil {
		return err
	}

	if err := tb.sendTelegramMessage(ctx, content); err != nil {
		return err
	}

	historyRecord := ChatHistory{UserID: ctx.EffectiveMessage.From.Id, UserName: ctx.EffectiveMessage.From.Username, UserMsg: message, BotMsg: content, LastUsed: time.Now()}
	if err := db.AddChatHistory(&historyRecord); err != nil {
		return err
	}

	return nil
}

func (tb *TelegramBot) handleMrlResetRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	logger.Info("Received MRL_RESET request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}

	if ctx.EffectiveMessage.From.Id != config.TelegramAdminUID {
		ctx.EffectiveMessage.Reply(b, "You are not authorized to use this command.", nil)
		return nil
	}

	if err := db.ClearChatHistory(); err != nil {
		return err
	}

	_, err := ctx.EffectiveMessage.Reply(b, "History has been reset.", nil)
	return err
}

func (tb *TelegramBot) sendTelegramMessage(ctx *ext.Context, text string) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := ctx.EffectiveMessage.Reply(tb.bot, text, nil)
	if err != nil {
		return err
	}
	return nil
}

func (tb *TelegramBot) forwardTelegramMessage(ctx *ext.Context, forwardChatID int64, forwardMessageID int64) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := tb.bot.ForwardMessage(ctx.EffectiveChat.Id, forwardChatID, forwardMessageID, nil)
	if err != nil {
		return err
	}
	return nil
}
