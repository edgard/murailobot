package main

import (
	"database/sql"
	"errors"
	"log"
	"math/rand"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tkanos/gonfig"
)

var userList []User

func sendMessagePiu(bot *tgbotapi.BotAPI, update tgbotapi.Update, db *sqlx.DB) {

	for i := range(userList) {
		if userList[i].UserID == update.Message.From.ID {
			if time.Since(userList[i].Timestamp).Minutes() <= 5 {
				return
			}
		}
    }
	userList = append(userList, User{update.Message.From.ID, time.Now()})

	var msgRef MessageReference
	err := db.Get(&msgRef, "SELECT id, message_id, chat_id FROM messages WHERE id IN (SELECT id FROM messages ORDER BY last_used ASC LIMIT 3) ORDER BY RANDOM() LIMIT 1")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Println("No results found, database empty?")
			return
		} else {
			log.Panic(err)
		}
	}

	_, err = db.Exec("UPDATE messages SET last_used=$1 WHERE id=$2", time.Now(), msgRef.ID)
	if err != nil {
		log.Println(err)
	}

	// for i := 0; i <= randomIntRange(1, 3); i++ {
	// 	var piu string
	// 	for x := 0; x <= randomIntRange(1, 4); x++ {
	// 		piu += "piu "
	// 	}
	// 	sendMessage(bot, update, piu)
	// 	time.Sleep(time.Duration(rand.Intn(3)) * time.Second)
	// }

	sendForward(bot, update, msgRef.ChatID, msgRef.MessageID)
}

func sendMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, text string) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Println(err)
	}
}

func sendForward(bot *tgbotapi.BotAPI, update tgbotapi.Update, forwardChatID int64, forwardMessageID int) {
	msg := tgbotapi.NewForward(update.Message.Chat.ID, forwardChatID, forwardMessageID)
	if _, err := bot.Send(msg); err != nil {
		log.Println(err)
	}
}

func processMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, db *sqlx.DB) {
	if update.Message.ForwardDate == 0 {
		log.Println("Not a forward, ignoring")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Println(err)
		return
	}
	_, err = tx.Exec("INSERT INTO messages (message_id, chat_id, last_used) VALUES ($1, $2, $3)", update.Message.MessageID, update.Message.Chat.ID, time.Now())
	if err != nil {
		tx.Rollback()
		log.Println(err)
		return
	}
	err = tx.Commit()
	if err != nil {
		log.Println(err)
		return
	}

	sendMessage(bot, update, "Mensagem adicionada ao banco de dados!")
}

// func randomIntRange(min int, max int) int {
// 	return rand.Intn(max-min+1) + min
// }

func main() {
	config := Configuration{}
	err := gonfig.GetConf("config.json", &config)
	if err != nil {
		log.Panic(err)
	}

	db, err := sqlx.Connect("sqlite3", config.DB)
	if err != nil {
		log.Panic(err)
	}
	db.MustExec(schema)

	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// initialize random seed
	rand.Seed(time.Now().UnixNano())

	// messages loop
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Println(err)
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if !update.Message.IsCommand() {
			processMessage(bot, update, db)
			continue
		}

		switch update.Message.Command() {
		case "piu":
			sendMessagePiu(bot, update, db)
		case "start":
			sendMessage(bot, update, "Olar! Me faça forward de uma mensagem para guardar.")
		}
	}
}
