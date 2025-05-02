// Package tasks implements scheduled tasks for the MurailoBot Telegram bot.
// It includes task definitions, dependencies, and registration mechanisms.
package tasks

import (
	"log/slog"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
	"github.com/edgard/murailobot/internal/gemini"
)

// TaskDeps contains all dependencies required by scheduled tasks.
// It provides access to logging, database, AI client, and configuration.
type TaskDeps struct {
	Logger       *slog.Logger
	Store        database.Store
	GeminiClient gemini.Client
	Config       *config.Config
}
