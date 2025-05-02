// Package database provides database access, migration management, and storage operations
// for the MurailoBot Telegram bot. It uses SQLite for persistent storage.
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

	// Import SQLite driver for database connectivity
	_ "modernc.org/sqlite"
)

// NewDB initializes and returns a new database connection with migrations applied.
// It connects to the SQLite database at the given path and runs any pending migrations.
func NewDB(dbPath string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s: %w", dbPath, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	dbName := ExtractDBNameFromPath(dbPath)
	if err := ApplyMigrations(db.DB, dbName); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Error closing database after migration failure", "close_error", closeErr, "migration_error", err)
		}
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	slog.Info("Database connected and migrations applied successfully", "path", dbPath)
	return db, nil
}

// CloseDB safely closes the database connection, logging any errors.
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

// ApplyMigrations runs database migrations to ensure the schema is up to date.
// It uses the embedded migration files to apply any pending schema changes.
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
		return fmt.Errorf("failed to create migration source driver: %w", err)
	}

	dbDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create sqlite3 migration database driver: %w", err)
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

	migrateErr := migrator.Up()

	if migrateErr != nil {
		if errors.Is(migrateErr, migrate.ErrNoChange) {
			slog.Info("No database migrations to apply (schema is up-to-date).")
			return nil
		}

		return fmt.Errorf("failed during migration application: %w", migrateErr)
	}

	slog.Info("Database migrations applied successfully.")
	return nil
}

// ExtractDBNameFromPath extracts the database name from a file path for migration purposes.
func ExtractDBNameFromPath(path string) string {
	path = strings.TrimPrefix(path, "file:")

	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}

	return path
}
