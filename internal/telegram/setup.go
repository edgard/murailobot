// Package telegram provides Telegram bot initialization and handler registration.
// It handles the integration with the Telegram Bot API using the go-telegram/bot library.
package telegram

import (
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"

	"github.com/edgard/murailobot/internal/bot/handlers"
)

// NewTelegramBot creates a new instance of the Telegram bot with the provided token and options.
// It sets up the bot with the default logger and any additional options specified.
func NewTelegramBot(token string, logger *slog.Logger, opts ...bot.Option) (*bot.Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token cannot be empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "telegram_bot")

	b, err := bot.New(token, opts...)
	if err != nil {
		log.Error("Failed to create Telegram bot instance", "error", err)
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	log.Info("Telegram bot instance created successfully", "token_prefix", token[:min(8, len(token))]+"...")
	return b, nil
}

func applyMiddleware(handler bot.HandlerFunc, mw []bot.Middleware) bot.HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// RegisterHandlers registers all command handlers with the Telegram bot.
// It adds the provided handlers to the bot with their patterns and middleware.
func RegisterHandlers(b *bot.Bot, logger *slog.Logger, registeredHandlers map[string]handlers.RegisteredHandler) error {
	if b == nil {
		return fmt.Errorf("bot instance cannot be nil for handler registration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With("component", "handler_registry")

	if len(registeredHandlers) == 0 {
		log.Warn("No handlers provided for registration.")
		return nil
	}

	log.Info("Registering Telegram handlers...", "count", len(registeredHandlers))

	for name, regHandler := range registeredHandlers {
		if regHandler.Handler == nil {
			log.Warn("Skipping registration for nil handler", "name", name, "pattern", regHandler.Pattern)
			continue
		}

		finalHandler := applyMiddleware(regHandler.Handler, regHandler.Middleware)

		b.RegisterHandler(regHandler.HandlerType, regHandler.Pattern, regHandler.MatchType, finalHandler)
		log.Debug("Registered handler",
			"name", name,
			"pattern", regHandler.Pattern,
			"type", regHandler.HandlerType,
			"match", regHandler.MatchType,
			"middleware_count", len(regHandler.Middleware))
	}

	log.Info("Registered Telegram handlers successfully", "count", len(registeredHandlers))
	return nil
}
