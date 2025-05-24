package handlers

import (
	tgbot "github.com/go-telegram/bot"
)

// RegisteredHandler represents a command handler with its description and middleware.
// It encapsulates all information needed to register and document a command.
type RegisteredHandler struct {
	HandlerType tgbot.HandlerType
	Pattern     string
	Handler     tgbot.HandlerFunc
	Middleware  []tgbot.Middleware
	MatchType   tgbot.MatchType
}

// RegisterAllCommands initializes and returns a map of all available bot commands.
// It configures each command with appropriate handlers and middleware.
func RegisterAllCommands(deps HandlerDeps) map[string]RegisteredHandler {
	handlers := make(map[string]RegisteredHandler)

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

	adminMiddleware := []tgbot.Middleware{AdminOnly(deps)}

	handlers["/mrl_reset"] = RegisteredHandler{
		HandlerType: tgbot.HandlerTypeMessageText,
		Pattern:     "mrl_reset",
		Handler:     NewResetHandler(deps),
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
