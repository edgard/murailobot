package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var config Config
var db *sqlx.DB
var bot *tgbotapi.BotAPI

func sendMessagePiu(update tgbotapi.Update) {
	var user User
	if err := db.Get(&user, "SELECT * FROM user WHERE user_id=$1", update.Message.From.ID); err != nil {
		if err == sql.ErrNoRows {
			log.Info().
				Int("user_id", update.Message.From.ID).
				Str("username", update.Message.From.UserName).
				Msg("User not found, creating user")
			user = User{UserID: update.Message.From.ID, LastUsed: time.Now().Add(-time.Minute * time.Duration(config.UserTimeout+1))}
			if _, err := db.NamedExec("INSERT INTO user (user_id, last_used) VALUES (:user_id, :last_used)", user); err != nil {
				log.Error().Err(err).Msg("Error creating user")
			}
		} else {
			log.Error().Err(err).Msg("Error querying for user")
			return
		}
	}

	if time.Since(user.LastUsed).Minutes() <= config.UserTimeout {
		log.Info().
			Int("user_id", user.UserID).
			Str("username", update.Message.From.UserName).
			Time("last_used", user.LastUsed).
			Msg("User on timeout")
		return
	}

	user.LastUsed = time.Now()
	if _, err := db.NamedExec("UPDATE user SET last_used=:last_used WHERE user_id=:user_id", user); err != nil {
		log.Error().Err(err).Msg("Error updating user last_used timestamp")
	}

	var msgRef MessageRef
	if err := db.Get(&msgRef, "SELECT * FROM message_ref WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5) ORDER BY RANDOM() LIMIT 1"); err != nil {
		log.Error().Err(err).Msg("Error getting message reference")
		return
	}

	msgRef.LastUsed = time.Now()
	if _, err := db.NamedExec("UPDATE message_ref SET last_used=:last_used WHERE id=:id", msgRef); err != nil {
		log.Error().Err(err).Msg("Error updating message reference last_used timestamp")
	}

	log.Info().
		Int64("msgref_chat_id", msgRef.ChatID).
		Int("msgref_message_id", msgRef.MessageID).
		Int64("chat_id", update.Message.Chat.ID).
		Int("user_id", update.Message.From.ID).
		Str("username", update.Message.From.UserName).
		Msg("Sending forward")

	sendForward(update, msgRef.ChatID, msgRef.MessageID)
}

func sendMessage(update tgbotapi.Update, text string) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Error().Err(err).Msg("Error sending message")
	}
}

func sendForward(update tgbotapi.Update, forwardChatID int64, forwardMessageID int) {
	msg := tgbotapi.NewForward(update.Message.Chat.ID, forwardChatID, forwardMessageID)
	if _, err := bot.Send(msg); err != nil {
		log.Error().Err(err).Msg("Error sending forward")
	}
}

func processMessage(update tgbotapi.Update) {
	if update.Message.ForwardDate == 0 {
		log.Info().
			Int("user_id", update.Message.From.ID).
			Str("username", update.Message.From.UserName).
			Int("update_id", update.UpdateID).Msg("Ignoring non-forward update")
		return
	}

	msgRef := MessageRef{MessageID: update.Message.MessageID, ChatID: update.Message.Chat.ID, LastUsed: time.Now()}
	if _, err := db.NamedExec("INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (:message_id, :chat_id, :last_used)", msgRef); err != nil {
		log.Error().Err(err).Msg("Error creating message reference")
		return
	}

	log.Info().Int("message_id", msgRef.MessageID).Int64("chat_id", msgRef.ChatID).Msg("Message reference created")
	sendMessage(update, "Mensagem adicionada ao banco de dados!")
}

func sendMessageGPT(update tgbotapi.Update) {
	message := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/mrl"))

	log.Info().
		Int("user_id", update.Message.From.ID).
		Str("username", update.Message.From.UserName).
		Int("update_id", update.UpdateID).Msg(fmt.Sprintf("Received GPT request: %s", message))

	// Fetch GPT history
	gptHist := []Hist{}
	if err := db.Select(&gptHist, "SELECT * FROM gpt_hist ORDER BY last_used DESC LIMIT 10"); err != nil {
		log.Error().Err(err).Msg("Error getting GPT history")
		sendMessage(update, "Ignorei")
		return
	}

	url := "https://api.openai.com/v1/chat/completions"

	var gptReq strings.Builder
	gptReq.WriteString(config.OpenAIInstruction)
	gptReq.WriteString("\n\nFor context to be used on the reply if needed, the last user requests and your replies are below, formatted as |Username|User Req|MurailoGPT (You) Reply|Timestamp|\n\n")
	for _, hist := range gptHist {
		gptReq.WriteString(fmt.Sprintf("|%s|%s|%s|%s|\n", hist.UserName, hist.UserMsg, hist.BotMsg, hist.LastUsed.Format("2006-01-02T15:04:05-0700")))
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "system", "content": gptReq.String()},
			{"role": "user", "content": message},
		},
	})

	if err != nil {
		handleError(update, err, "Unable to marshall request")
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		handleError(update, err, "Unable to create request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.OpenAIToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		handleError(update, err, "Unable to get response")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		handleError(update, err, "Unable to read response")
		return
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		handleError(update, err, "Unable to unmarshal response")
		return
	}

	if len(response.Choices) > 0 {
		content := response.Choices[0].Message.Content
		sendMessage(update, content)
		histRef := Hist{UserName: update.Message.From.UserName, UserMsg: message, BotMsg: content, LastUsed: time.Now()}
		if _, err := db.NamedExec("INSERT INTO gpt_hist (user_name, user_msg, bot_msg, last_used) VALUES (:user_name, :user_msg, :bot_msg, :last_used)", histRef); err != nil {
			log.Error().Err(err).Msg("Error creating history reference")
		} else {
			log.Info().Str("user_name", histRef.UserName).Str("user_msg", histRef.UserMsg).Msg("History reference created")
		}
		return
	}

	handleError(update, fmt.Errorf("unexpected msg format"), "Unexpected msg format")
}

func handleError(update tgbotapi.Update, err error, msg string) {
	sendMessage(update, "Ignorei")
	log.Error().Err(err).Msg(msg)
}

func init() {
	// init config
	err := envconfig.Process("murailobot", &config)
	if err != nil {
		log.Fatal().Err(err).Msg("Error reading environment config variables")
	}

	// open db
	db, err = sqlx.Connect("sqlite3", config.DBName)
	if err != nil {
		log.Fatal().Err(err).Msg("Error opening database")
	}

	// exec the db schema
	if _, err := db.Exec(dbSchema); err != nil {
		log.Fatal().Err(err).Msg("Error executing database schema")
	}

	// init bot
	bot, err = tgbotapi.NewBotAPI(config.AuthToken)
	if err != nil {
		log.Fatal().Err(err).Msg("Error logging in to Telegram")
	}
	bot.Debug = config.TelegramDebug
}

func main() {
	log.Info().Str("username", bot.Self.UserName).Msg("Logged in to Telegram")

	// initialize messages loop
	u := tgbotapi.NewUpdate(0)
	u.Timeout = config.UpdateTimeout

	// messages loop
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Warn().Err(err).Msg("Error getting updates")
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if !update.Message.IsCommand() {
			processMessage(update)
			continue
		}

		switch update.Message.Command() {
		case "piu":
			sendMessagePiu(update)
		case "start":
			sendMessage(update, "Ola! Me encaminhe uma mensagem para guardar.")
		case "mrl":
			sendMessageGPT(update)
		}
	}
}
