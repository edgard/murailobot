package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/resilience"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// New creates a new SQLite database connection with the given configuration
func New(cfg *Config) (*DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: configuration is nil", ErrDatabase)
	}

	// Validate database configuration
	switch {
	case cfg.Name == "":
		return nil, fmt.Errorf("%w: database name is required", ErrDatabase)
	case cfg.MaxOpenConns < 1:
		return nil, fmt.Errorf("%w: max_open_conns must be at least 1, got %d", ErrDatabase, cfg.MaxOpenConns)
	case cfg.MaxOpenConns > 100:
		return nil, fmt.Errorf("%w: max_open_conns must not exceed 100, got %d", ErrDatabase, cfg.MaxOpenConns)
	case cfg.MaxIdleConns < 0:
		return nil, fmt.Errorf("%w: max_idle_conns must not be negative, got %d", ErrDatabase, cfg.MaxIdleConns)
	case cfg.MaxIdleConns > cfg.MaxOpenConns:
		return nil, fmt.Errorf("%w: max_idle_conns (%d) must not exceed max_open_conns (%d)",
			ErrDatabase, cfg.MaxIdleConns, cfg.MaxOpenConns)
	case cfg.ConnMaxLifetime < time.Second:
		return nil, fmt.Errorf("%w: conn_max_lifetime must be at least 1 second, got %v",
			ErrDatabase, cfg.ConnMaxLifetime)
	case cfg.ConnMaxLifetime > 24*time.Hour:
		return nil, fmt.Errorf("%w: conn_max_lifetime must not exceed 24 hours, got %v",
			ErrDatabase, cfg.ConnMaxLifetime)
	}

	// Ensure database directory exists with proper permissions
	dbDir := filepath.Dir(cfg.Name)
	if dbDir != "." {
		// Create directory with secure permissions
		if err := os.MkdirAll(dbDir, 0750); err != nil {
			return nil, fmt.Errorf("%w: failed to create database directory: %v", ErrDatabase, err)
		}

		// Verify directory permissions
		info, err := os.Stat(dbDir)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to check directory permissions: %v", ErrDatabase, err)
		}

		// Ensure directory has secure permissions
		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(dbDir, 0750); err != nil {
				return nil, fmt.Errorf("%w: failed to set secure directory permissions: %v", ErrDatabase, err)
			}
		}
	}

	// Check if database file exists and verify permissions
	if info, err := os.Stat(cfg.Name); err == nil {
		// Ensure database file has secure permissions (readable/writable only by owner)
		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(cfg.Name, 0600); err != nil {
				return nil, fmt.Errorf("%w: failed to set secure database file permissions: %v", ErrDatabase, err)
			}
		}
	}

	// Configure circuit breaker for database operations
	breaker := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:          "sqlite-db",
		MaxFailures:   3,
		Timeout:       5 * time.Second,
		HalfOpenLimit: 1,
		ResetInterval: 10 * time.Second,
		OnStateChange: func(name string, from, to resilience.CircuitState) {
			slog.Info("Database circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	// Set up connection with retry
	var conn *sqlx.DB
	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.WithRetry(ctx, func(ctx context.Context) error {
			dsn := fmt.Sprintf("%s?_journal=WAL&_foreign_keys=on&_busy_timeout=5000&_secure_delete=on", cfg.Name)
			var err error
			conn, err = sqlx.ConnectContext(ctx, "sqlite3", dsn)
			return err
		}, resilience.DefaultRetryConfig())
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	db := &DB{
		DB:      conn,
		config:  cfg,
		breaker: breaker,
	}

	if err := db.setupSchema(); err != nil {
		conn.Close()
		return nil, err
	}

	slog.Info("database initialized",
		"name", cfg.Name,
		"max_open_conns", cfg.MaxOpenConns,
		"max_idle_conns", cfg.MaxIdleConns,
		"conn_max_lifetime", cfg.ConnMaxLifetime,
		"journal_mode", "WAL",
		"busy_timeout", "5000ms",
	)
	return db, nil
}

func (db *DB) setupSchema() error {
	// Use context with timeout for schema setup
	ctx, cancel := context.WithTimeout(context.Background(), defaultLongOperationTimeout)
	defer cancel()

	// Set PRAGMAs before starting transaction
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("%w: failed to set pragma %q: %v", ErrDatabase, pragma, err)
		}
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to start transaction: %v", ErrDatabase, err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Error("failed to rollback schema setup transaction", "error", err)
		}
	}()

	schemas := []string{
		getChatHistoryTableSchema(db.config.MaxMessageSize),
		createChatHistoryTimestampIndex,
		createChatHistoryUserIDIndex,
	}

	for i, schema := range schemas {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("%w: failed to execute schema %d: %v", ErrDatabase, i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: failed to commit schema changes: %v", ErrDatabase, err)
	}

	// Verify schema setup
	var tableCount int
	if err := db.GetContext(ctx, &tableCount, "SELECT count(*) FROM sqlite_master WHERE type='table'"); err != nil {
		return fmt.Errorf("%w: failed to verify schema setup: %v", ErrDatabase, err)
	}

	if tableCount < 2 {
		return fmt.Errorf("%w: schema verification failed: expected 2 tables, got %d", ErrDatabase, tableCount)
	}

	// Verify foreign key constraints are enabled
	var foreignKeys bool
	if err := db.GetContext(ctx, &foreignKeys, "PRAGMA foreign_keys"); err != nil {
		return fmt.Errorf("%w: failed to verify foreign keys: %v", ErrDatabase, err)
	}

	if !foreignKeys {
		return fmt.Errorf("%w: foreign key constraints are not enabled", ErrDatabase)
	}

	slog.Info("database schema initialized",
		"tables", tableCount,
		"foreign_keys", foreignKeys,
		"journal_mode", "WAL",
		"synchronous", "NORMAL",
		"busy_timeout", "5000ms",
	)
	return nil
}

// GetRecentChatHistory retrieves the last n messages from chat history
func (db *DB) GetRecentChatHistory(ctx context.Context, limit int) ([]ChatHistory, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: invalid limit: %d", ErrDatabase, limit)
	}

	// Cap the limit to prevent excessive memory usage
	if limit > 50 {
		limit = 50
		slog.Warn("limiting chat history retrieval", "requested", limit, "max_allowed", 50)
	}

	var history []ChatHistory
	err := db.breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.WithRetry(ctx, func(ctx context.Context) error {
			query := `SELECT id, user_id, user_name, user_msg, bot_msg, timestamp
				FROM chat_history
				WHERE user_msg != '' AND bot_msg != ''
				ORDER BY timestamp DESC
				LIMIT ?`

			rows, err := db.QueryxContext(ctx, query, limit)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var msg ChatHistory
				if err := rows.StructScan(&msg); err != nil {
					return fmt.Errorf("failed to scan chat history: %v", err)
				}
				history = append(history, msg)
			}

			if err := rows.Err(); err != nil {
				return fmt.Errorf("error iterating chat history: %v", err)
			}

			return nil
		}, resilience.DefaultRetryConfig())
	})

	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return nil, fmt.Errorf("%w: circuit breaker is open", ErrDatabase)
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	if len(history) == 0 {
		slog.Debug("no chat history found", "limit", limit)
		return nil, nil
	}

	slog.Debug("retrieved chat history",
		"limit", limit,
		"messages_found", len(history),
	)
	return history, nil
}

// SaveChatInteraction saves a new chat message to the database
func (db *DB) SaveChatInteraction(ctx context.Context, userID int64, userName, userMsg, botMsg string) error {
	if userID <= 0 {
		return fmt.Errorf("%w: user_id must be positive", ErrDatabase)
	}

	// Validate and trim username
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return fmt.Errorf("%w: username cannot be empty", ErrDatabase)
	}
	if len(userName) > 64 { // reasonable max length for username
		userName = userName[:64]
	}

	// Validate message lengths
	if len(userMsg) == 0 || len(botMsg) == 0 {
		return fmt.Errorf("%w: messages cannot be empty", ErrDatabase)
	}
	if len(userMsg) > db.config.MaxMessageSize || len(botMsg) > db.config.MaxMessageSize {
		return fmt.Errorf("%w: message exceeds maximum length of %d characters", ErrDatabase, db.config.MaxMessageSize)
	}

	err := db.breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.WithRetry(ctx, func(ctx context.Context) error {
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to start transaction: %v", err)
			}
			defer func() {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					slog.Error("failed to rollback save chat transaction", "error", err)
				}
			}()

			now := time.Now()

			result, err := tx.ExecContext(ctx,
				`INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, timestamp)
				VALUES (?, ?, ?, ?, ?)`,
				userID, userName, userMsg, botMsg, now,
			)
			if err != nil {
				return fmt.Errorf("failed to save chat: %v", err)
			}

			messageID, err := result.LastInsertId()
			if err != nil {
				return fmt.Errorf("failed to get message ID: %v", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit transaction: %v", err)
			}

			slog.Debug("chat history saved",
				"message_id", messageID,
				"user_id", userID,
				"user_name", userName,
				"user_msg_length", len(userMsg),
				"bot_msg_length", len(botMsg),
				"timestamp", now.Format(time.RFC3339),
			)
			return nil
		}, resilience.DefaultRetryConfig())
	})

	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return fmt.Errorf("%w: circuit breaker is open", ErrDatabase)
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	return nil
}

// DeleteAllChatHistory deletes all chat history from the database
func (db *DB) DeleteAllChatHistory(ctx context.Context) error {
	err := db.breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.WithRetry(ctx, func(ctx context.Context) error {
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to start transaction: %v", err)
			}
			defer func() {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					slog.Error("failed to rollback clear history transaction", "error", err)
				}
			}()

			if _, err := tx.ExecContext(ctx, "DELETE FROM chat_history"); err != nil {
				return fmt.Errorf("failed to clear chat history: %v", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit transaction: %v", err)
			}

			slog.Info("chat history cleared successfully")
			return nil
		}, resilience.DefaultRetryConfig())
	})

	if err != nil {
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return fmt.Errorf("%w: circuit breaker is open", ErrDatabase)
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if err := db.DB.Close(); err != nil {
		return fmt.Errorf("%w: failed to close database connection: %v", ErrDatabase, err)
	}
	return nil
}
