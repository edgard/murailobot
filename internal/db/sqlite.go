// Package db provides SQLite-based persistence for chat interactions.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/utils"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sony/gobreaker"
)

// SQL statements for creating indices to optimize query performance.
var (
	// createChatHistoryTimestampIndex creates an index on the timestamp column
	// to optimize retrieval of recent messages.
	createChatHistoryTimestampIndex = `
		CREATE INDEX IF NOT EXISTS idx_chat_history_timestamp ON chat_history(timestamp)`

	// createChatHistoryUserIDIndex creates an index on the user_id column
	// to optimize user-specific queries.
	createChatHistoryUserIDIndex = `
		CREATE INDEX IF NOT EXISTS idx_chat_history_user_id ON chat_history(user_id)`
)

// sqliteDB implements the Database interface using SQLite as the backend.
// It provides thread-safe access to the database with connection pooling,
// circuit breaking, and automatic retries for transient failures.
type sqliteDB struct {
	*sqlx.DB
	config   *config.Config
	dbConfig *config.DatabaseConfig
	breaker  *utils.CircuitBreaker
}

const componentName = "db"

// New creates a new SQLite database connection with the given configuration.
// It performs the following setup:
// 1. Creates and secures the database directory
// 2. Sets up connection pooling and circuit breaker
// 3. Initializes the database schema
// 4. Configures SQLite for optimal performance
func New(cfg *config.Config) (Database, error) {
	if cfg == nil {
		return nil, utils.NewError(componentName, utils.ErrInvalidConfig, "configuration is nil", utils.CategoryConfig, nil)
	}

	dbConfig := &cfg.Database

	// Ensure database directory exists with proper permissions (0750)
	dbDir := filepath.Dir(dbConfig.Name)
	if dbDir != "." {
		if err := os.MkdirAll(dbDir, 0750); err != nil {
			return nil, utils.NewError(componentName, utils.ErrOperation, "failed to create database directory", utils.CategoryOperation, err)
		}

		info, err := os.Stat(dbDir)
		if err != nil {
			return nil, utils.NewError(componentName, utils.ErrOperation, "failed to check directory permissions", utils.CategoryOperation, err)
		}

		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(dbDir, 0750); err != nil {
				return nil, utils.NewError(componentName, utils.ErrOperation, "failed to set secure directory permissions", utils.CategoryOperation, err)
			}
		}
	}

	// Ensure database file has secure permissions (0600)
	if info, err := os.Stat(dbConfig.Name); err == nil {
		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(dbConfig.Name, 0600); err != nil {
				return nil, utils.NewError(componentName, utils.ErrOperation, "failed to set secure database file permissions", utils.CategoryOperation, err)
			}
		}
	}

	// Configure circuit breaker for database operations
	breaker := utils.NewCircuitBreaker(utils.CircuitBreakerConfig{
		Name:          "sqlite-db",
		MaxFailures:   3,
		Timeout:       5 * time.Second,
		HalfOpenLimit: 1,
		ResetInterval: 10 * time.Second,
		OnStateChange: func(name string, from, to utils.CircuitState) {
			utils.WriteInfoLog(componentName, "circuit breaker state changed",
				utils.KeyName, name,
				utils.KeyFrom, from.String(),
				utils.KeyTo, to.String(),
				utils.KeyType, "circuit_breaker",
				utils.KeyAction, "state_change")
		},
	})

	// Set up connection with retry
	var conn *sqlx.DB
	ctx, cancel := context.WithTimeout(context.Background(), dbConfig.OperationTimeout)
	defer cancel()

	err := breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			dsn := fmt.Sprintf("%s?_journal=%s&_foreign_keys=%s&_busy_timeout=5000&_secure_delete=on",
				dbConfig.Name,
				dbConfig.JournalMode,
				boolToOnOff(dbConfig.ForeignKeys))
			var err error
			conn, err = sqlx.ConnectContext(ctx, "sqlite3", dsn)
			if err != nil {
				return utils.NewError(componentName, utils.ErrConnection, "failed to connect to database", utils.CategoryExternal, err)
			}
			return nil
		}, utils.DefaultRetryConfig())
	})

	if err != nil {
		return nil, err
	}

	// Configure connection pool for optimal concurrency
	conn.SetMaxOpenConns(dbConfig.MaxOpenConns)
	conn.SetMaxIdleConns(dbConfig.MaxIdleConns)
	conn.SetConnMaxLifetime(dbConfig.ConnMaxLifetime)

	utils.WriteDebugLog(componentName, "database connection pool configured",
		utils.KeyAction, "configure_pool",
		utils.KeyType, "sqlite",
		"pool_config", map[string]interface{}{
			"max_open_conns":    dbConfig.MaxOpenConns,
			"max_idle_conns":    dbConfig.MaxIdleConns,
			"conn_max_lifetime": dbConfig.ConnMaxLifetime,
		})

	db := &sqliteDB{
		DB:       conn,
		config:   cfg,
		dbConfig: dbConfig,
		breaker:  breaker,
	}

	if err := db.setupSchema(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

// getPragmas returns SQLite pragma statements based on configuration.
// These pragmas optimize SQLite for the bot's usage patterns:
// - journal_mode: Controls how write transactions are journaled
// - synchronous: Controls fsync behavior for durability vs performance
// - foreign_keys: Enables foreign key constraint enforcement
// - temp_store: Controls temporary table and index storage location
// - cache_size: Controls the page cache size in memory
func getPragmas(cfg *config.DatabaseConfig) []string {
	// Convert cache size from KB to pages (each page is 1KB)
	cacheSize := -cfg.CacheSizeKB // Negative value means KB instead of number of pages

	return []string{
		"PRAGMA journal_mode=" + cfg.JournalMode,
		"PRAGMA synchronous=" + cfg.Synchronous,
		"PRAGMA foreign_keys=" + boolToOnOff(cfg.ForeignKeys),
		"PRAGMA temp_store=" + cfg.TempStore,
		"PRAGMA cache_size=" + strconv.Itoa(cacheSize),
	}
}

// boolToOnOff converts a boolean to SQLite's "ON" or "OFF" string representation.
func boolToOnOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// getChatHistoryTableSchema returns the chat history table schema.
// The schema includes:
// - Automatic ID generation
// - User identification fields
// - Message content with length constraints
// - Timestamp for message ordering
func getChatHistoryTableSchema(maxMessageSize int) string {
	return `
		CREATE TABLE IF NOT EXISTS chat_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_name TEXT NOT NULL,
			user_msg TEXT NOT NULL CHECK(length(user_msg) <= ` + strconv.Itoa(maxMessageSize) + `),
			bot_msg TEXT NOT NULL CHECK(length(bot_msg) <= ` + strconv.Itoa(maxMessageSize) + `),
			timestamp DATETIME NOT NULL
		)`
}

// setupSchema initializes the database schema and configures SQLite settings.
// It runs in a transaction to ensure atomic schema creation and verification.
func (s *sqliteDB) setupSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.dbConfig.LongOperationTimeout)
	defer cancel()

	utils.WriteDebugLog(componentName, "setting database pragmas",
		utils.KeyAction, "set_pragmas",
		utils.KeyType, "sqlite",
		"pragmas", map[string]interface{}{
			"journal_mode":  s.dbConfig.JournalMode,
			"synchronous":   s.dbConfig.Synchronous,
			"foreign_keys":  s.dbConfig.ForeignKeys,
			"temp_store":    s.dbConfig.TempStore,
			"cache_size_kb": s.dbConfig.CacheSizeKB,
		})

	for _, pragma := range getPragmas(s.dbConfig) {
		if _, err := s.ExecContext(ctx, pragma); err != nil {
			return utils.Errorf(componentName, utils.ErrOperation, utils.CategoryOperation,
				"failed to set pragma %q: %v", pragma, err)
		}
	}

	tx, err := s.BeginTxx(ctx, nil)
	if err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to start transaction", utils.CategoryOperation, err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			utils.WriteErrorLog(componentName, "failed to rollback schema setup transaction", err,
				utils.KeyAction, "rollback",
				utils.KeyTxType, "schema_setup")
		}
	}()

	schemas := []string{
		getChatHistoryTableSchema(s.config.MaxMessageSize),
		createChatHistoryTimestampIndex,
		createChatHistoryUserIDIndex,
	}

	for i, schema := range schemas {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return utils.Errorf(componentName, utils.ErrOperation, utils.CategoryOperation,
				"failed to execute schema %d: %v", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to commit schema changes", utils.CategoryOperation, err)
	}

	// Verify schema setup
	var tableCount int
	if err := s.GetContext(ctx, &tableCount, "SELECT count(*) FROM sqlite_master WHERE type='table'"); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to verify schema setup", utils.CategoryOperation, err)
	}

	if tableCount < 1 {
		return utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
			"schema verification failed: expected at least 1 table, got %d", tableCount)
	}

	// Verify foreign key constraints are enabled
	var foreignKeys bool
	if err := s.GetContext(ctx, &foreignKeys, "PRAGMA foreign_keys"); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to verify foreign keys", utils.CategoryOperation, err)
	}

	if !foreignKeys {
		return utils.NewError(componentName, utils.ErrValidation, "foreign key constraints are not enabled", utils.CategoryValidation, nil)
	}

	utils.WriteInfoLog(componentName, "database initialized",
		utils.KeyAction, "initialize",
		utils.KeyResult, "success",
		utils.KeyName, s.dbConfig.Name,
		utils.KeyType, "sqlite",
		utils.KeyCount, tableCount,
		"settings", map[string]interface{}{
			"connections": map[string]interface{}{
				"max_open":     s.dbConfig.MaxOpenConns,
				"max_idle":     s.dbConfig.MaxIdleConns,
				"max_lifetime": s.dbConfig.ConnMaxLifetime,
			},
			"timeouts": map[string]interface{}{
				"operation":      s.dbConfig.OperationTimeout,
				"long_operation": s.dbConfig.LongOperationTimeout,
			},
			"pragmas": map[string]interface{}{
				"journal_mode":  s.dbConfig.JournalMode,
				"synchronous":   s.dbConfig.Synchronous,
				"foreign_keys":  foreignKeys,
				"temp_store":    s.dbConfig.TempStore,
				"cache_size_kb": s.dbConfig.CacheSizeKB,
			},
		})
	return nil
}

// GetRecentChatHistory retrieves the most recent chat interactions.
// It enforces a maximum limit of 50 messages to prevent excessive memory usage.
// Results are ordered by timestamp descending (newest first) and exclude empty messages.
func (s *sqliteDB) GetRecentChatHistory(ctx context.Context, limit int) ([]ChatHistory, error) {
	if limit <= 0 {
		return nil, utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
			"invalid limit: %d", limit)
	}

	if limit > 50 {
		limit = 50
		utils.WriteWarnLog(componentName, "limiting chat history retrieval",
			utils.KeyRequested, limit,
			utils.KeyLimit, 50,
			utils.KeyAction, "get_history",
			utils.KeyType, "chat_history")
	}

	var history []ChatHistory
	err := s.breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			query := `SELECT id, user_id, user_name, user_msg, bot_msg, timestamp
				FROM chat_history
				WHERE user_msg != '' AND bot_msg != ''
				ORDER BY timestamp DESC
				LIMIT ?`

			rows, err := s.QueryxContext(ctx, query, limit)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to query chat history", utils.CategoryOperation, err)
			}
			defer rows.Close()

			for rows.Next() {
				var msg ChatHistory
				if err := rows.StructScan(&msg); err != nil {
					return utils.NewError(componentName, utils.ErrOperation, "failed to scan chat history", utils.CategoryOperation, err)
				}
				history = append(history, msg)
			}

			if err := rows.Err(); err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "error iterating chat history", utils.CategoryOperation, err)
			}

			return nil
		}, utils.DefaultRetryConfig())
	})

	if err != nil {
		if stderrors.Is(err, gobreaker.ErrOpenState) {
			return nil, utils.NewError(componentName, utils.ErrOperation, "circuit breaker is open", utils.CategoryOperation, err)
		}
		return nil, err
	}

	if len(history) == 0 {
		utils.WriteDebugLog(componentName, "no chat history found",
			utils.KeyLimit, limit,
			utils.KeyAction, "get_history",
			utils.KeyType, "chat_history")
		return nil, nil
	}

	utils.WriteDebugLog(componentName, "retrieved chat history",
		utils.KeyLimit, limit,
		utils.KeyCount, len(history),
		utils.KeyAction, "get_history",
		utils.KeyType, "chat_history")
	return history, nil
}

// SaveChatInteraction stores a new chat interaction in the database.
// It performs the following validations:
// - User ID must be positive
// - Username must not be empty (truncated if too long)
// - Messages must not be empty or exceed size limits
// The operation is performed in a transaction with retry and circuit breaking.
func (s *sqliteDB) SaveChatInteraction(ctx context.Context, userID int64, userName, userMsg, botMsg string) error {
	if userID <= 0 {
		return utils.NewError(componentName, utils.ErrValidation, "user_id must be positive", utils.CategoryValidation, nil)
	}

	userName = strings.TrimSpace(userName)
	if userName == "" {
		return utils.NewError(componentName, utils.ErrValidation, "username cannot be empty", utils.CategoryValidation, nil)
	}
	if len(userName) > s.dbConfig.MaxUsernameLen {
		userName = userName[:s.dbConfig.MaxUsernameLen]
	}

	if len(userMsg) == 0 || len(botMsg) == 0 {
		return utils.NewError(componentName, utils.ErrValidation, "messages cannot be empty", utils.CategoryValidation, nil)
	}
	if len(userMsg) > s.config.MaxMessageSize || len(botMsg) > s.config.MaxMessageSize {
		return utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
			"message exceeds maximum length of %d characters", s.config.MaxMessageSize)
	}

	err := s.breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			tx, err := s.BeginTxx(ctx, nil)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to start transaction", utils.CategoryOperation, err)
			}
			defer func() {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					utils.WriteErrorLog(componentName, "failed to rollback save chat transaction", err,
						utils.KeyAction, "rollback",
						utils.KeyTxType, "save_chat")
				}
			}()

			now := time.Now()

			result, err := tx.ExecContext(ctx,
				`INSERT INTO chat_history (user_id, user_name, user_msg, bot_msg, timestamp)
				VALUES (?, ?, ?, ?, ?)`,
				userID, userName, userMsg, botMsg, now,
			)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to save chat", utils.CategoryOperation, err)
			}

			messageID, err := result.LastInsertId()
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to get message ID", utils.CategoryOperation, err)
			}

			if err := tx.Commit(); err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to commit transaction", utils.CategoryOperation, err)
			}

			utils.WriteDebugLog(componentName, "chat history saved",
				utils.KeyRequestID, messageID,
				utils.KeyUserID, userID,
				utils.KeyName, userName,
				utils.KeySize, len(userMsg)+len(botMsg),
				utils.KeyAction, "save_chat",
				utils.KeyType, "chat_history",
				"timestamp", now.Format(time.RFC3339))
			return nil
		}, utils.DefaultRetryConfig())
	})

	if err != nil {
		if stderrors.Is(err, gobreaker.ErrOpenState) {
			return utils.NewError(componentName, utils.ErrOperation, "circuit breaker is open", utils.CategoryOperation, err)
		}
		return err
	}

	return nil
}

// DeleteAllChatHistory removes all chat history from the database.
// This operation is performed in a transaction and cannot be undone.
// It uses circuit breaking and retry mechanisms for reliability.
func (s *sqliteDB) DeleteAllChatHistory(ctx context.Context) error {
	err := s.breaker.Execute(ctx, func(ctx context.Context) error {
		return utils.WithRetry(ctx, func(ctx context.Context) error {
			tx, err := s.BeginTxx(ctx, nil)
			if err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to start transaction", utils.CategoryOperation, err)
			}
			defer func() {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					utils.WriteErrorLog(componentName, "failed to rollback clear history transaction", err,
						utils.KeyAction, "rollback",
						utils.KeyTxType, "clear_history")
				}
			}()

			if _, err := tx.ExecContext(ctx, "DELETE FROM chat_history"); err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to clear chat history", utils.CategoryOperation, err)
			}

			if err := tx.Commit(); err != nil {
				return utils.NewError(componentName, utils.ErrOperation, "failed to commit transaction", utils.CategoryOperation, err)
			}

			utils.WriteInfoLog(componentName, "chat history cleared",
				utils.KeyAction, "clear_history",
				utils.KeyResult, "success",
				utils.KeyType, "chat_history")
			return nil
		}, utils.DefaultRetryConfig())
	})

	if err != nil {
		if stderrors.Is(err, gobreaker.ErrOpenState) {
			return utils.NewError(componentName, utils.ErrOperation, "circuit breaker is open", utils.CategoryOperation, err)
		}
		return err
	}

	return nil
}

// Close releases all database resources.
// After closing, no other methods should be called on this instance.
func (s *sqliteDB) Close() error {
	if err := s.DB.Close(); err != nil {
		return utils.NewError(componentName, utils.ErrOperation, "failed to close database connection", utils.CategoryOperation, err)
	}
	return nil
}
