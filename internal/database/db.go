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

	"github.com/edgard/murailobot-go/migrations"

	_ "modernc.org/sqlite" //revive:disable:blank-imports
)

// NewDB initializes, applies migrations, and returns a new database connection pool.
// dbPath should be a path to the SQLite database file.
func NewDB(dbPath string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// SQLite-specific connection pool settings
	// SQLite doesn't support concurrent writes, so max open conns = 1
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	dbName := ExtractDBNameFromPath(dbPath)
	if err := ApplyMigrations(db.DB, dbName); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Error closing database after migration failure", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	slog.Info("Database connected and migrations applied successfully", "path", dbPath)
	return db, nil
}

// CloseDB closes the database connection pool.
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

// ApplyMigrations runs database migrations using embedded files.
func ApplyMigrations(db *sql.DB, dbName string) error {
	if db == nil {
		return errors.New("database connection is nil, cannot apply migrations")
	}
	if dbName == "" {
		return errors.New("database name/path for migration driver is empty")
	}

	slog.Info("Applying database migrations...", "database_name", dbName)

	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create embed source driver instance: %w", err)
	}

	dbDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create sqlite3 database driver: %w", err)
	}
	migrator, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"sqlite3",
		dbDriver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Fix: Store migration result in a separate variable to avoid confusion in the subsequent conditions
	migrateErr := migrator.Up()

	// Handle migration errors
	if migrateErr != nil {
		if errors.Is(migrateErr, migrate.ErrNoChange) {
			// Not a real error, just nothing to do
			slog.Info("No database migrations to apply.")
			return nil
		}
		return fmt.Errorf("failed to apply migrations: %w", migrateErr)
	}

	// Only log here if migrations were actually applied
	slog.Info("Database migrations applied successfully.")
	return nil
}

// ExtractDBNameFromPath extracts the database file path from a possibly URL-formatted path.
// This handles both simple file paths and paths with URL-style encoding.
func ExtractDBNameFromPath(path string) string {
	// Remove file: prefix if present
	path = strings.TrimPrefix(path, "file:")

	// Remove URL query parameters if present
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// URL-decode if necessary
	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}

	return path
}
