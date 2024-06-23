package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// User represents a user in the database.
type User struct {
	UserID   int64     // Unique identifier for the user
	LastUsed time.Time // Timestamp of the last time the user was active
}

// MessageRef represents a message reference in the database.
type MessageRef struct {
	ID        uint      // Unique identifier for the message reference
	MessageID int64     // ID of the message
	ChatID    int64     // ID of the chat
	LastUsed  time.Time // Timestamp of the last time the message reference was used
}

// ChatHistory represents chat history in the database.
type ChatHistory struct {
	ID       uint      // Unique identifier for the chat history entry
	UserID   int64     // ID of the user
	UserName string    // Name of the user
	UserMsg  string    // Message sent by the user
	BotMsg   string    // Message sent by the bot
	LastUsed time.Time // Timestamp of the last time the chat history entry was used
}

// DB implements the database interactions using SQLite.
type DB struct {
	conn *sql.DB // Database connection
}

// NewDB initializes the database connection and schema.
func NewDB(config *Config) (*DB, error) {
	conn, err := sql.Open("sqlite3", config.DBName)
	if err != nil {
		return nil, WrapError("failed to connect to database", err)
	}

	db := &DB{conn: conn}
	err = db.setupSchema()
	if err != nil {
		return nil, WrapError("failed to set up database schema", err)
	}
	return db, nil
}

// setupSchema creates the necessary tables if they don't already exist.
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
		return WrapError("failed to execute schema setup", err)
	}
	return nil
}

// GetOrCreateUser fetches a user from the database or creates one if not found.
func (db *DB) GetOrCreateUser(userID int64, timeout float64) (User, error) {
	var user User
	query := "SELECT user_id, last_used FROM user WHERE user_id = ?"
	insertQuery := "INSERT INTO user (user_id, last_used) VALUES (?, ?)"

	err := db.conn.QueryRow(query, userID).Scan(&user.UserID, &user.LastUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			user = User{
				UserID:   userID,
				LastUsed: time.Now().Add(-time.Minute * time.Duration(timeout+1)),
			}
			_, err := db.conn.Exec(insertQuery, user.UserID, user.LastUsed)
			if err != nil {
				return user, WrapError("failed to insert new user", err)
			}
		} else {
			return user, WrapError("failed to fetch user", err)
		}
	}
	return user, nil
}

// UpdateUserLastUsed updates the last used timestamp for a user.
func (db *DB) UpdateUserLastUsed(user User) error {
	user.LastUsed = time.Now()
	query := "UPDATE user SET last_used = ? WHERE user_id = ?"
	_, err := db.conn.Exec(query, user.LastUsed, user.UserID)
	if err != nil {
		return WrapError("failed to update user last used time", err)
	}
	return nil
}

// GetRandomMessageRef retrieves a random message reference from the database.
func (db *DB) GetRandomMessageRef() (MessageRef, error) {
	var msgRef MessageRef
	selectQuery := `
		SELECT id, message_id, chat_id, last_used
		FROM message_ref
		WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5)
		ORDER BY RANDOM()
		LIMIT 1`
	updateQuery := "UPDATE message_ref SET last_used = ? WHERE id = ?"

	err := db.conn.QueryRow(selectQuery).Scan(&msgRef.ID, &msgRef.MessageID, &msgRef.ChatID, &msgRef.LastUsed)
	if err != nil {
		return msgRef, WrapError("failed to retrieve random message reference", err)
	}

	msgRef.LastUsed = time.Now()
	_, err = db.conn.Exec(updateQuery, msgRef.LastUsed, msgRef.ID)
	if err != nil {
		return msgRef, WrapError("failed to update message reference last used time", err)
	}
	return msgRef, nil
}

// AddMessageRef inserts a new message reference into the database.
func (db *DB) AddMessageRef(msgRef *MessageRef) error {
	query := "INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (?, ?, ?)"
	_, err := db.conn.Exec(query, msgRef.MessageID, msgRef.ChatID, msgRef.LastUsed)
	if err != nil {
		return WrapError("failed to add message reference", err)
	}
	return nil
}

// GetRecentChatHistory retrieves recent chat history from the database.
func (db *DB) GetRecentChatHistory(limit int) ([]ChatHistory, error) {
	query := `
		SELECT id, user_id, user_name, user_msg, bot_msg, last_used
		FROM chat_history
		ORDER BY last_used DESC
		LIMIT ?`
	rows, err := db.conn.Query(query, limit)
	if err != nil {
		return nil, WrapError("failed to retrieve recent chat history", err)
	}
	defer rows.Close()

	var history []ChatHistory
	for rows.Next() {
		var entry ChatHistory
		err := rows.Scan(&entry.ID, &entry.UserID, &entry.UserName, &entry.UserMsg, &entry.BotMsg, &entry.LastUsed)
		if err != nil {
			return nil, WrapError("failed to scan chat history", err)
		}
		history = append(history, entry)
	}

	err = rows.Err()
	if err != nil {
		return nil, WrapError("rows iteration error", err)
	}
	return history, nil
}

// AddChatHistory inserts new chat history into the database.
func (db *DB) AddChatHistory(history *ChatHistory) error {
	query := "INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, last_used) VALUES (?, ?, ?, ?, ?)"
	_, err := db.conn.Exec(query, history.UserID, history.UserName, history.UserMsg, history.BotMsg, history.LastUsed)
	if err != nil {
		return WrapError("failed to add chat history", err)
	}
	return nil
}

// ClearChatHistory deletes all chat history from the database.
func (db *DB) ClearChatHistory() error {
	query := "DELETE FROM chat_history"
	_, err := db.conn.Exec(query)
	if err != nil {
		return WrapError("failed to clear chat history", err)
	}
	return nil
}
