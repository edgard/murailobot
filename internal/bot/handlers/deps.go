package handlers

import (
	"log/slog"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/database"
	"github.com/edgard/murailobot/internal/gemini"
)

// HandlerDeps provides dependencies for Telegram command handlers.
type HandlerDeps struct {
	Logger       *slog.Logger
	Config       *config.Config
	Store        database.Store
	GeminiClient gemini.Client
}
