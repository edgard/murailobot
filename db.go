package main

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// User represents a user in the database.
type User struct {
	UserID   int64     `db:"user_id"`
	LastUsed time.Time `db:"last_used"`
}

// MessageRef represents a message reference in the database.
type MessageRef struct {
	ID        uint      `db:"id"`
	MessageID int64     `db:"message_id"`
	ChatID    int64     `db:"chat_id"`
	LastUsed  time.Time `db:"last_used"`
}

// ChatHistory represents chat history in the database.
type ChatHistory struct {
	ID       uint      `db:"id"`
	UserID   int64     `db:"user_id"`
	UserName string    `db:"user_name"`
	UserMsg  string    `db:"user_msg"`
	BotMsg   string    `db:"bot_msg"`
	LastUsed time.Time `db:"last_used"`
}

// Database implements the database interactions using SQLite.
type DB struct {
	conn *sqlx.DB
}

// NewDB initializes the database connection and schema.
func NewDB(dataSourceName string) (*DB, error) {
	conn, err := sqlx.Connect("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.setupSchema(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) setupSchema() error {
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
		user_id INTEGER,
		user_name TEXT,
		user_msg TEXT,
		bot_msg TEXT,
		last_used DATETIME
	);`
	_, err := db.conn.Exec(schema)
	return err
}

// GetOrCreateUser fetches a user from the database or creates one if not found.
func (db *DB) GetOrCreateUser(userID int64, timeout float64) (User, error) {
	var user User
	err := db.conn.Get(&user, "SELECT * FROM user WHERE user_id = ?", userID)
	if err != nil {
		if err == sql.ErrNoRows {
			user = User{
				UserID:   userID,
				LastUsed: time.Now().Add(-time.Minute * time.Duration(timeout+1)),
			}
			_, err = db.conn.NamedExec("INSERT INTO user (user_id, last_used) VALUES (:user_id, :last_used)", &user)
			if err != nil {
				return user, err
			}
		} else {
			return user, err
		}
	}
	return user, nil
}

// UpdateUserLastUsed updates the last used timestamp for a user.
func (db *DB) UpdateUserLastUsed(user User) error {
	user.LastUsed = time.Now()
	_, err := db.conn.NamedExec("UPDATE user SET last_used = :last_used WHERE user_id = :user_id", &user)
	return err
}

// GetRandomMessageRef retrieves a random message reference from the database.
func (db *DB) GetRandomMessageRef() (MessageRef, error) {
	var msgRef MessageRef
	err := db.conn.Get(&msgRef, "SELECT * FROM message_ref WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5) ORDER BY RANDOM() LIMIT 1")
	if err != nil {
		return msgRef, err
	}
	msgRef.LastUsed = time.Now()
	_, err = db.conn.NamedExec("UPDATE message_ref SET last_used = :last_used WHERE id = :id", &msgRef)
	if err != nil {
		return msgRef, err
	}
	return msgRef, nil
}

// AddMessageRef inserts a new message reference into the database.
func (db *DB) AddMessageRef(msgRef *MessageRef) error {
	_, err := db.conn.NamedExec("INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (:message_id, :chat_id, :last_used)", msgRef)
	return err
}

// GetRecentChatHistory retrieves recent chat history from the database.
func (db *DB) GetRecentChatHistory(limit int) ([]ChatHistory, error) {
	var history []ChatHistory
	err := db.conn.Select(&history, "SELECT * FROM chat_history ORDER BY last_used DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	return history, nil
}

// AddChatHistory inserts new chat history into the database.
func (db *DB) AddChatHistory(history *ChatHistory) error {
	_, err := db.conn.NamedExec("INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, last_used) VALUES (:user_id, :user_name, :user_msg, :bot_msg, :last_used)", history)
	return err
}

// ClearChatHistory deletes all chat history from the database.
func (db *DB) ClearChatHistory() error {
	_, err := db.conn.Exec("DELETE FROM chat_history")
	return err
}
