// Package tasks provides interfaces and dependencies for scheduled background tasks.
package tasks

import (
	"log/slog"

	"github.com/edgard/murailobot-go/internal/config"
	"github.com/edgard/murailobot-go/internal/database"
	"github.com/edgard/murailobot-go/internal/gemini"
)

// TaskDeps provides dependencies for scheduled tasks.
type TaskDeps struct {
	Logger       *slog.Logger
	Config       *config.Config
	Store        database.Store
	GeminiClient gemini.Client
}
