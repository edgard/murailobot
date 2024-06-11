package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"go.uber.org/zap"
)

var bot *gotgbot.Bot

func initTelegramBot() error {
	var err error
	bot, err = gotgbot.NewBot(config.TelegramToken, nil)
	if err != nil {
		return err
	}
	return nil
}

func startTelegramBot() {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			logger.Warn("Error occurred while handling update", zap.Error(err))
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	dispatcher.AddHandler(handlers.NewCommand("start", handleStartRequest))
	dispatcher.AddHandler(handlers.NewCommand("piu", handlePiuRequest))
	dispatcher.AddHandler(handlers.NewCommand("mrl", handleMrlRequest))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, handleIncomingMessage))

	err := updater.StartPolling(bot, &ext.PollingOpts{
		DropPendingUpdates: true,
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

	logger.Info("Started Telegram Bot", zap.String("username", bot.User.Username))

	updater.Idle()
}

func handleIncomingMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage.ForwardOrigin == nil {
		logger.Info("Received non-forward message, ignoring", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))
		return nil
	}

	logger.Info("Received forward message", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	msgRef := MessageRef{MessageID: ctx.EffectiveMessage.MessageId, ChatID: ctx.EffectiveMessage.Chat.Id, LastUsed: time.Now()}
	err := insertMessageRef(&msgRef)
	if err != nil {
		return err
	}

	return sendTelegramMessage(ctx, "Mensagem adicionada ao banco de dados!")
}

func handleStartRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	return sendTelegramMessage(ctx, "Olá! Me encaminhe uma mensagem para guardar.")
}

func handlePiuRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	logger.Info("Received PIU request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	user, err := getUserOrCreate(ctx)
	if err != nil {
		return err
	}

	if time.Since(user.LastUsed).Minutes() <= config.UserTimeout {
		logger.Info("User on timeout", zap.Int64("user_id", user.UserID), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Time("last_used", user.LastUsed))
		return nil
	}

	if err := updateUserLastUsed(user); err != nil {
		return err
	}

	msgRef, err := getRandomMessageRef()
	if err != nil {
		return err
	}

	return forwardTelegramMessage(ctx, msgRef.ChatID, msgRef.MessageID)
}

func handleMrlRequest(b *gotgbot.Bot, ctx *ext.Context) error {
	logger.Info("Received GPT request", zap.Int64("user_id", ctx.EffectiveMessage.From.Id), zap.String("username", ctx.EffectiveMessage.From.Username), zap.Int64("update_id", ctx.Update.UpdateId))

	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}

	message := strings.TrimSpace(strings.TrimPrefix(ctx.EffectiveMessage.Text, "/mrl"))

	gptHistory, err := getRecentChatHistory(20)
	if err != nil {
		return err
	}

	appConfig, err := getAppConfig()
	if err != nil {
		appConfig.OpenAIInstruction = "You are MurailoGPT, an AI assistant that provides sarcastic responses."
	}

	url := "https://api.openai.com/v1/chat/completions"

	messages := []map[string]string{
		{"role": "system", "content": appConfig.OpenAIInstruction},
	}

	sort.Slice(gptHistory, func(i, j int) bool {
		return gptHistory[i].LastUsed.Before(gptHistory[j].LastUsed)
	})

	for _, history := range gptHistory {
		userName := history.UserName
		if userName == "" {
			userName = "Unknown User"
		}
		messages = append(messages, map[string]string{
			"role": "user", "content": fmt.Sprintf("%s [%s]: %s", userName, history.LastUsed.Format(time.RFC3339), history.UserMsg),
		})
		messages = append(messages, map[string]string{
			"role": "assistant", "content": history.BotMsg,
		})
	}

	messages = append(messages, map[string]string{
		"role": "user", "content": fmt.Sprintf("%s [%s]: %s", ctx.EffectiveMessage.From.Username, time.Now().Format(time.RFC3339), message),
	})

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    "gpt-4o",
		"messages": messages,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.OpenAIToken))

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}

	if len(response.Choices) > 0 {
		content := response.Choices[0].Message.Content
		if err := sendTelegramMessage(ctx, content); err != nil {
			return err
		}
		historyRecord := ChatHistory{UserName: ctx.EffectiveMessage.From.Username, UserMsg: message, BotMsg: content, LastUsed: time.Now()}
		if err := insertChatHistory(&historyRecord); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("unexpected message format")
}

func sendTelegramMessage(ctx *ext.Context, text string) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := ctx.EffectiveMessage.Reply(bot, text, nil)
	if err != nil {
		return err
	}
	return nil
}

func forwardTelegramMessage(ctx *ext.Context, forwardChatID int64, forwardMessageID int64) error {
	if ctx.EffectiveMessage == nil {
		return fmt.Errorf("ctx.EffectiveMessage is nil")
	}
	_, err := bot.ForwardMessage(ctx.EffectiveChat.Id, forwardChatID, forwardMessageID, nil)
	if err != nil {
		return err
	}
	return nil
}
