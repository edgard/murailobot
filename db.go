package main

import (
	"database/sql"
	"fmt"
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
func NewDB(config *Config) (*DB, error) {
	conn, err := sqlx.Connect("sqlite3", config.DBName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.setupSchema(); err != nil {
		return nil, fmt.Errorf("failed to set up database schema: %w", err)
	}
	return db, nil
}

func (db *DB) setupSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_ref (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id INTEGER NOT NULL,
		chat_id INTEGER NOT NULL,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS user (
		user_id INTEGER PRIMARY KEY,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		user_name TEXT NOT NULL,
		user_msg TEXT NOT NULL,
		bot_msg TEXT NOT NULL,
		last_used DATETIME
	);`
	_, err := db.conn.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema setup: %w", err)
	}
	return nil
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
				return user, fmt.Errorf("failed to insert new user: %w", err)
			}
		} else {
			return user, fmt.Errorf("failed to fetch user: %w", err)
		}
	}
	return user, nil
}

// UpdateUserLastUsed updates the last used timestamp for a user.
func (db *DB) UpdateUserLastUsed(user User) error {
	user.LastUsed = time.Now()
	_, err := db.conn.NamedExec("UPDATE user SET last_used = :last_used WHERE user_id = :user_id", &user)
	if err != nil {
		return fmt.Errorf("failed to update user last used time: %w", err)
	}
	return nil
}

// GetRandomMessageRef retrieves a random message reference from the database.
func (db *DB) GetRandomMessageRef() (MessageRef, error) {
	var msgRef MessageRef
	err := db.conn.Get(&msgRef, "SELECT * FROM message_ref WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5) ORDER BY RANDOM() LIMIT 1")
	if err != nil {
		return msgRef, fmt.Errorf("failed to retrieve random message reference: %w", err)
	}
	msgRef.LastUsed = time.Now()
	_, err = db.conn.NamedExec("UPDATE message_ref SET last_used = :last_used WHERE id = :id", &msgRef)
	if err != nil {
		return msgRef, fmt.Errorf("failed to update message reference last used time: %w", err)
	}
	return msgRef, nil
}

// AddMessageRef inserts a new message reference into the database.
func (db *DB) AddMessageRef(msgRef *MessageRef) error {
	_, err := db.conn.NamedExec("INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (:message_id, :chat_id, :last_used)", msgRef)
	if err != nil {
		return fmt.Errorf("failed to add message reference: %w", err)
	}
	return nil
}

// GetRecentChatHistory retrieves recent chat history from the database.
func (db *DB) GetRecentChatHistory(limit int) ([]ChatHistory, error) {
	var history []ChatHistory
	err := db.conn.Select(&history, "SELECT * FROM chat_history ORDER BY last_used DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve recent chat history: %w", err)
	}
	return history, nil
}

// AddChatHistory inserts new chat history into the database.
func (db *DB) AddChatHistory(history *ChatHistory) error {
	_, err := db.conn.NamedExec("INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, last_used) VALUES (:user_id, :user_name, :user_msg, :bot_msg, :last_used)", history)
	if err != nil {
		return fmt.Errorf("failed to add chat history: %w", err)
	}
	return nil
}

// ClearChatHistory deletes all chat history from the database.
func (db *DB) ClearChatHistory() error {
	_, err := db.conn.Exec("DELETE FROM chat_history")
	if err != nil {
		return fmt.Errorf("failed to clear chat history: %w", err)
	}
	return nil
}
