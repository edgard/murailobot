package main

import (
	"database/sql"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// MessageRef represents a message reference in the database
type MessageRef struct {
	ID        uint      `db:"id"`
	MessageID int64     `db:"message_id"`
	ChatID    int64     `db:"chat_id"`
	LastUsed  time.Time `db:"last_used"`
}

// User represents a user in the database
type User struct {
	UserID   int64     `db:"user_id"`
	LastUsed time.Time `db:"last_used"`
}

// ChatHistory represents chat history in the database
type ChatHistory struct {
	ID       uint      `db:"id"`
	UserName string    `db:"user_name"`
	UserMsg  string    `db:"user_msg"`
	BotMsg   string    `db:"bot_msg"`
	LastUsed time.Time `db:"last_used"`
}

// AppConfig represents the application configuration in the database
type AppConfig struct {
	ID                uint   `db:"id"`
	OpenAIInstruction string `db:"openai_instruction"`
}

var db *sqlx.DB

func initDatabase() error {
	var err error
	db, err = sqlx.Connect("sqlite3", config.DBName)
	if err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS message_ref (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id INTEGER,
		chat_id INTEGER,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS user (
		user_id INTEGER PRIMARY KEY,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_name TEXT,
		user_msg TEXT,
		bot_msg TEXT,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS app_config (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		openai_instruction TEXT
	);`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return nil
}

// User-related functions
func getUserOrCreate(ctx *ext.Context) (User, error) {
	var user User
	err := db.Get(&user, "SELECT * FROM user WHERE user_id = ?", ctx.EffectiveMessage.From.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			user = User{
				UserID:   ctx.EffectiveMessage.From.Id,
				LastUsed: time.Now().Add(-time.Minute * time.Duration(config.UserTimeout+1)),
			}
			_, err = db.NamedExec("INSERT INTO user (user_id, last_used) VALUES (:user_id, :last_used)", &user)
			if err != nil {
				return user, err
			}
		} else {
			return user, err
		}
	}
	return user, nil
}

func updateUserLastUsed(user User) error {
	user.LastUsed = time.Now()
	_, err := db.NamedExec("UPDATE user SET last_used = :last_used WHERE user_id = :user_id", &user)
	return err
}

// Message reference-related functions
func getRandomMessageRef() (MessageRef, error) {
	var msgRef MessageRef
	err := db.Get(&msgRef, "SELECT * FROM message_ref WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5) ORDER BY RANDOM() LIMIT 1")
	if err != nil {
		return msgRef, err
	}
	msgRef.LastUsed = time.Now()
	_, err = db.NamedExec("UPDATE message_ref SET last_used = :last_used WHERE id = :id", &msgRef)
	if err != nil {
		return msgRef, err
	}
	return msgRef, nil
}

func insertMessageRef(msgRef *MessageRef) error {
	_, err := db.NamedExec("INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (:message_id, :chat_id, :last_used)", msgRef)
	return err
}

// Chat history-related functions
func getRecentChatHistory(limit int) ([]ChatHistory, error) {
	var history []ChatHistory
	err := db.Select(&history, "SELECT * FROM chat_history ORDER BY last_used DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func insertChatHistory(history *ChatHistory) error {
	_, err := db.NamedExec("INSERT INTO chat_history (user_name, user_msg, bot_msg, last_used) VALUES (:user_name, :user_msg, :bot_msg, :last_used)", history)
	return err
}

// App configuration-related functions
func getAppConfig() (AppConfig, error) {
	var appConfig AppConfig
	err := db.Get(&appConfig, "SELECT * FROM app_config ORDER BY id LIMIT 1")
	if err != nil {
		return appConfig, err
	}
	return appConfig, nil
}

func insertAppConfig(instruction string) error {
	_, err := db.Exec("INSERT INTO app_config (openai_instruction) VALUES (?)", instruction)
	return err
}

func updateAppConfig(instruction string, id uint) error {
	_, err := db.Exec("UPDATE app_config SET openai_instruction = ? WHERE id = ?", instruction, id)
	return err
}
