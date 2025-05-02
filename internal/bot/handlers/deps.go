package handlers

import (
	"log/slog"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
	"github.com/edgard/murailobot/internal/gemini"
)

// HandlerDeps contains all dependencies required by the handlers.
// It provides access to logging, database, AI client, and configuration.
type HandlerDeps struct {
	Logger       *slog.Logger
	Config       *config.Config
	Store        database.Store
	GeminiClient gemini.Client
}
