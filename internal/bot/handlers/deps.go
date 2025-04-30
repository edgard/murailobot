package handlers

import (
	"log/slog"

	"github.com/edgard/murailobot-go/internal/config"
	"github.com/edgard/murailobot-go/internal/database"
	"github.com/edgard/murailobot-go/internal/gemini"
)

// HandlerDeps provides dependencies for Telegram command handlers.
type HandlerDeps struct {
	Logger       *slog.Logger
	Config       *config.Config
	Store        database.Store
	GeminiClient gemini.Client
}
