package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// User represents a user in the database.
type User struct {
	UserID   int64     // Unique identifier.
	LastUsed time.Time // Last activity timestamp.
}

// MessageRef represents a stored message reference.
type MessageRef struct {
	ID        uint      // Unique identifier.
	MessageID int64     // Telegram message ID.
	ChatID    int64     // Telegram chat ID.
	LastUsed  time.Time // Last time this reference was used.
}

// ChatHistory represents an entry in the chat history.
type ChatHistory struct {
	ID       uint      // Unique identifier.
	UserID   int64     // User ID.
	UserName string    // Username.
	UserMsg  string    // User's message.
	BotMsg   string    // Bot's reply.
	LastUsed time.Time // Timestamp of the entry.
}

// DB wraps the SQLite connection.
type DB struct {
	conn *sql.DB
}

// NewDB opens a connection and sets up the schema.
func NewDB(config *Config) (*DB, error) {
	conn, err := sql.Open("sqlite3", config.DBName)
	if err != nil {
		return nil, WrapError("failed to connect to database", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, WrapError("failed to ping database", err)
	}

	db := &DB{conn: conn}
	if err := db.setupSchema(); err != nil {
		return nil, WrapError("failed to set up database schema", err)
	}
	return db, nil
}

// setupSchema creates the necessary tables.
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
	if _, err := db.conn.Exec(schema); err != nil {
		return WrapError("failed to execute schema setup", err)
	}
	return nil
}

// GetOrCreateUser retrieves or creates a user record.
func (db *DB) GetOrCreateUser(userID int64, timeout float64) (User, error) {
	var user User
	query := "SELECT user_id, last_used FROM user WHERE user_id = ?"
	insertQuery := "INSERT INTO user (user_id, last_used) VALUES (?, ?)"
	err := db.conn.QueryRow(query, userID).Scan(&user.UserID, &user.LastUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			pastTime := time.Now().Add(-time.Minute * time.Duration(int(timeout)+1))
			user = User{UserID: userID, LastUsed: pastTime}
			if _, err := db.conn.Exec(insertQuery, user.UserID, user.LastUsed); err != nil {
				return user, WrapError("failed to insert new user", err)
			}
		} else {
			return user, WrapError("failed to fetch user", err)
		}
	}
	return user, nil
}

// UpdateUserLastUsed updates a user's last used timestamp.
func (db *DB) UpdateUserLastUsed(user User) error {
	user.LastUsed = time.Now()
	query := "UPDATE user SET last_used = ? WHERE user_id = ?"
	if _, err := db.conn.Exec(query, user.LastUsed, user.UserID); err != nil {
		return WrapError("failed to update user last used time", err)
	}
	return nil
}

// GetRandomMessageRef retrieves a random message reference.
func (db *DB) GetRandomMessageRef() (MessageRef, error) {
	var msgRef MessageRef
	selectQuery := `
		SELECT id, message_id, chat_id, last_used
		FROM message_ref
		WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5)
		ORDER BY RANDOM()
		LIMIT 1`
	updateQuery := "UPDATE message_ref SET last_used = ? WHERE id = ?"
	if err := db.conn.QueryRow(selectQuery).Scan(&msgRef.ID, &msgRef.MessageID, &msgRef.ChatID, &msgRef.LastUsed); err != nil {
		return msgRef, WrapError("failed to retrieve random message reference", err)
	}
	msgRef.LastUsed = time.Now()
	if _, err := db.conn.Exec(updateQuery, msgRef.LastUsed, msgRef.ID); err != nil {
		return msgRef, WrapError("failed to update message reference last used time", err)
	}
	return msgRef, nil
}

// AddMessageRef inserts a new message reference.
func (db *DB) AddMessageRef(msgRef *MessageRef) error {
	query := "INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (?, ?, ?)"
	if _, err := db.conn.Exec(query, msgRef.MessageID, msgRef.ChatID, msgRef.LastUsed); err != nil {
		return WrapError("failed to add message reference", err)
	}
	return nil
}

// GetRecentChatHistory retrieves recent chat history entries.
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
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.UserName, &entry.UserMsg, &entry.BotMsg, &entry.LastUsed); err != nil {
			return nil, WrapError("failed to scan chat history", err)
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapError("rows iteration error", err)
	}
	return history, nil
}

// AddChatHistory inserts a new chat history entry.
func (db *DB) AddChatHistory(history *ChatHistory) error {
	query := "INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, last_used) VALUES (?, ?, ?, ?, ?)"
	if _, err := db.conn.Exec(query, history.UserID, history.UserName, history.UserMsg, history.BotMsg, history.LastUsed); err != nil {
		return WrapError("failed to add chat history", err)
	}
	return nil
}

// ClearChatHistory deletes all chat history records.
func (db *DB) ClearChatHistory() error {
	query := "DELETE FROM chat_history"
	if _, err := db.conn.Exec(query); err != nil {
		return WrapError("failed to clear chat history", err)
	}
	return nil
}
