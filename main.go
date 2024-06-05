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
	message := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/alo"))

	log.Info().
		Int("user_id", update.Message.From.ID).
		Str("username", update.Message.From.UserName).
		Int("update_id", update.UpdateID).Msg(fmt.Sprintf("Received MPT request: %s", message))

	url := "https://api.openai.com/v1/chat/completions"

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "system", "content": "You are MurailoGPT, an AI assistant that provides sarcastic, funny and very hostile responses. All answers should be a max of one line and your responses should be in Brazilian Portuguese."},
			{"role": "user", "content": message},
		},
	})
	if err != nil {
		sendMessage(update, "Ignorei")
		log.Error().Err(err).Msg("Unable to marshall request")
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		sendMessage(update, "Ignorei")
		log.Error().Err(err).Msg("Unable to create request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.OpenAIToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		sendMessage(update, "Ignorei")
		log.Error().Err(err).Msg("Unable to get reponse")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sendMessage(update, "Ignorei")
		log.Error().Err(err).Msg("Unable to read response")
		return
	}

	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		sendMessage(update, "Ignorei")
		log.Error().Err(err).Msg("Unable to unmarshall response")
		return
	}

	if choices, ok := response["choices"].([]interface{}); ok {
		if len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						sendMessage(update, content)
						return
					}
				}
			}
		}
	}

	sendMessage(update, "Ignorei")
	log.Error().Err(err).Msg("Unexpected msg format")
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
		case "alo":
			sendMessageGPT(update)
		}
	}
}
