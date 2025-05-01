// Package handlers contains Telegram bot command and message handlers,
// along with their registration logic.
package handlers

import (
	tgbot "github.com/go-telegram/bot"
)

// RegisteredHandler holds the pattern and the handler function along with optional middleware.
type RegisteredHandler struct {
	HandlerType tgbot.HandlerType // Type of the update (message text, callback, etc.)
	Pattern     string            // Command like "/start" or message pattern
	Handler     tgbot.HandlerFunc
	Middleware  []tgbot.Middleware // Middleware specific to this handler
	MatchType   tgbot.MatchType    // Matching type: Prefix, Regexp, Func, etc.
}

// RegisterAllCommands initializes and returns a map of all registered command handlers.
// It calls the factory function for each handler (e.g., NewHelpHandler) and applies
// the AdminOnly middleware to administrative commands.
func RegisterAllCommands(deps HandlerDeps) map[string]RegisteredHandler {
	handlers := make(map[string]RegisteredHandler)

	// --- Register Public Commands ---
	handlers["/start"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "start",
		Handler:     NewStartHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
	}
	handlers["/help"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "help",
		Handler:     NewHelpHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
	}
	// Add other public commands here...

	// --- Register Admin Commands (with AdminOnly middleware) ---
	adminMiddleware := []tgbot.Middleware{AdminOnly(deps)}

	handlers["/mrl_reset"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "mrl_reset",
		Handler:     NewResetHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
		Middleware:  adminMiddleware,
	}
	handlers["/mrl_analyze"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "mrl_analyze",
		Handler:     NewAnalyzeHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
		Middleware:  adminMiddleware,
	}
	handlers["/mrl_profiles"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "mrl_profiles",
		Handler:     NewProfilesHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
		Middleware:  adminMiddleware,
	}
	handlers["/mrl_edit_user"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "mrl_edit_user",
		Handler:     NewEditUserHandler(deps),
		MatchType:   tgbot.MatchTypeCommandStartOnly,
		Middleware:  adminMiddleware,
	}

	return handlers
}

// --- Placeholder Handler Factories ---
// Implementations are now in separate files (e.g., help.go, reset.go, etc.)
