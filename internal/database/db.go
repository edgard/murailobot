// Package database provides database setup, models, and data access layer (Store).
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	"github.com/edgard/murailobot/migrations"

	_ "modernc.org/sqlite" //revive:disable:blank-imports // SQLite driver
)

// NewDB initializes, applies migrations, and returns a new database connection pool.
// dbPath should be a path to the SQLite database file.
func NewDB(dbPath string) (*sqlx.DB, error) {
	// The "sqlite" driver name corresponds to modernc.org/sqlite
	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s: %w", dbPath, err)
	}

	// Configure SQLite-specific connection pool settings.
	// SQLite generally doesn't handle concurrent writes well across connections,
	// so limiting the pool size is crucial for stability.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)                  // Must be <= MaxOpenConns
	db.SetConnMaxLifetime(5 * time.Minute) // Close connections after 5 mins to release resources

	// Extract the base filename for the migration driver instance name
	dbName := ExtractDBNameFromPath(dbPath)
	if err := ApplyMigrations(db.DB, dbName); err != nil {
		// Attempt to close DB connection if migrations fail
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Error closing database after migration failure", "close_error", closeErr, "migration_error", err)
		}
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	slog.Info("Database connected and migrations applied successfully", "path", dbPath)
	return db, nil
}

// CloseDB closes the database connection pool and logs the outcome.
func CloseDB(db *sqlx.DB) {
	if db == nil {
		return
	}
	if err := db.Close(); err != nil {
		slog.Error("Error closing database connection", "error", err)
	} else {
		slog.Info("Database connection closed successfully.")
	}
}

// ApplyMigrations runs database migrations using embedded SQL files from the migrations package.
// It uses the golang-migrate library with an iofs source and sqlite3 database driver.
func ApplyMigrations(db *sql.DB, dbName string) error {
	if db == nil {
		return errors.New("database connection is nil, cannot apply migrations")
	}
	// dbName is used internally by the migrate driver, often for locking
	if dbName == "" {
		return errors.New("database name/path for migration driver is empty")
	}

	slog.Info("Applying database migrations...", "database_name", dbName)

	// Create migration source from embedded filesystem
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source driver: %w", err)
	}

	// Create migration database driver instance
	dbDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create sqlite3 migration database driver: %w", err)
	}

	// Create the migrator instance
	migrator, err := migrate.NewWithInstance(
		"iofs", // Source driver name
		sourceDriver,
		"sqlite3", // Database driver name
		dbDriver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Apply migrations upwards (apply all pending migrations)
	// Store migration result in a separate variable to check specific errors below.
	migrateErr := migrator.Up()

	// Handle migration results
	if migrateErr != nil {
		if errors.Is(migrateErr, migrate.ErrNoChange) {
			// ErrNoChange is not a failure; it means the schema is already up-to-date.
			slog.Info("No database migrations to apply (schema is up-to-date).")
			return nil
		}
		// Any other error is a real failure.
		return fmt.Errorf("failed during migration application: %w", migrateErr)
	}

	// Log success only if migrations were actually applied (i.e., no error and not ErrNoChange)
	slog.Info("Database migrations applied successfully.")
	return nil
}

// ExtractDBNameFromPath extracts the database file path from a possibly URL-formatted DSN.
// This handles both simple file paths (e.g., "data.db") and DSNs
// (e.g., "file:data.db?cache=shared&_pragma=foreign_keys(1)").
func ExtractDBNameFromPath(path string) string {
	// 1. Remove "file:" prefix if present
	path = strings.TrimPrefix(path, "file:")

	// 2. Remove URL query parameters (anything after '?')
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// 3. URL-decode the remaining path in case it contains encoded characters
	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}

	// Fallback to the processed path if decoding fails (should be rare)
	return path
}
