package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sqlx.DB connection.
type DB struct {
	Conn *sqlx.DB
}

// NewDB opens a connection to the SQLite database and sets up the schema.
func NewDB(dbName string) (*DB, error) {
	conn, err := sqlx.Connect("sqlite3", dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := setupSchema(conn); err != nil {
		return nil, err
	}
	return &DB{Conn: conn}, nil
}

func setupSchema(db *sqlx.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_ref (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id INTEGER NOT NULL,
		chat_id INTEGER NOT NULL,
		last_used DATETIME
	);
	CREATE TABLE IF NOT EXISTS "user" (
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
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to setup schema: %w", err)
	}
	return nil
}

// User represents a user record.
type User struct {
	UserID   int64     `db:"user_id"`
	LastUsed time.Time `db:"last_used"`
}

// MessageRef represents a stored message reference.
type MessageRef struct {
	ID        int64     `db:"id"`
	MessageID int64     `db:"message_id"`
	ChatID    int64     `db:"chat_id"`
	LastUsed  time.Time `db:"last_used"`
}

// ChatHistory represents a chat history entry.
type ChatHistory struct {
	ID       int64     `db:"id"`
	UserID   int64     `db:"user_id"`
	UserName string    `db:"user_name"`
	UserMsg  string    `db:"user_msg"`
	BotMsg   string    `db:"bot_msg"`
	LastUsed time.Time `db:"last_used"`
}

// GetOrCreateUser retrieves a user record or creates one if not found.
func (d *DB) GetOrCreateUser(ctx context.Context, userID int64, timeout float64) (User, error) {
	var user User
	query := `SELECT user_id, last_used FROM "user" WHERE user_id = ?`
	err := d.Conn.GetContext(ctx, &user, query, userID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			pastTime := time.Now().Add(-time.Minute * time.Duration(int(timeout)+1))
			user = User{UserID: userID, LastUsed: pastTime}
			insertQuery := `INSERT INTO "user" (user_id, last_used) VALUES (?, ?)`
			if _, err := d.Conn.ExecContext(ctx, insertQuery, user.UserID, user.LastUsed); err != nil {
				return user, fmt.Errorf("failed to insert new user: %w", err)
			}
		} else {
			return user, fmt.Errorf("failed to fetch user: %w", err)
		}
	}
	return user, nil
}

// UpdateUserLastUsed updates the last used timestamp for a user.
func (d *DB) UpdateUserLastUsed(ctx context.Context, user User) error {
	user.LastUsed = time.Now()
	query := `UPDATE "user" SET last_used = ? WHERE user_id = ?`
	if _, err := d.Conn.ExecContext(ctx, query, user.LastUsed, user.UserID); err != nil {
		return fmt.Errorf("failed to update user last used: %w", err)
	}
	return nil
}

// GetRandomMessageRef retrieves a random message reference from the least-used ones.
func (d *DB) GetRandomMessageRef(ctx context.Context) (MessageRef, error) {
	var msgRef MessageRef
	selectQuery := `
		SELECT id, message_id, chat_id, last_used
		FROM message_ref
		WHERE id IN (SELECT id FROM message_ref ORDER BY last_used ASC LIMIT 5)
		ORDER BY RANDOM()
		LIMIT 1`
	if err := d.Conn.GetContext(ctx, &msgRef, selectQuery); err != nil {
		return msgRef, fmt.Errorf("failed to get random message_ref: %w", err)
	}
	msgRef.LastUsed = time.Now()
	updateQuery := `UPDATE message_ref SET last_used = ? WHERE id = ?`
	if _, err := d.Conn.ExecContext(ctx, updateQuery, msgRef.LastUsed, msgRef.ID); err != nil {
		return msgRef, fmt.Errorf("failed to update message_ref: %w", err)
	}
	return msgRef, nil
}

// AddMessageRef inserts a new message reference.
func (d *DB) AddMessageRef(ctx context.Context, msgRef MessageRef) error {
	query := `INSERT INTO message_ref (message_id, chat_id, last_used) VALUES (?, ?, ?)`
	if _, err := d.Conn.ExecContext(ctx, query, msgRef.MessageID, msgRef.ChatID, msgRef.LastUsed); err != nil {
		return fmt.Errorf("failed to add message_ref: %w", err)
	}
	return nil
}

// GetRecentChatHistory retrieves recent chat history entries.
func (d *DB) GetRecentChatHistory(ctx context.Context, limit int) ([]ChatHistory, error) {
	var histories []ChatHistory
	query := `
		SELECT id, user_id, user_name, user_msg, bot_msg, last_used
		FROM chat_history
		ORDER BY last_used DESC
		LIMIT ?`
	if err := d.Conn.SelectContext(ctx, &histories, query, limit); err != nil {
		return nil, fmt.Errorf("failed to get chat_history: %w", err)
	}
	return histories, nil
}

// AddChatHistory inserts a new chat history entry.
func (d *DB) AddChatHistory(ctx context.Context, history ChatHistory) error {
	query := `INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, last_used) VALUES (?, ?, ?, ?, ?)`
	if _, err := d.Conn.ExecContext(ctx, query, history.UserID, history.UserName, history.UserMsg, history.BotMsg, history.LastUsed); err != nil {
		return fmt.Errorf("failed to add chat_history: %w", err)
	}
	return nil
}

// ClearChatHistory deletes all chat history records.
func (d *DB) ClearChatHistory(ctx context.Context) error {
	query := `DELETE FROM chat_history`
	if _, err := d.Conn.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to clear chat_history: %w", err)
	}
	return nil
}
