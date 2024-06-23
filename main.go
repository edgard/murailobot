package main

import (
	"github.com/rs/zerolog/log"
)

// App encapsulates the entire application.
type App struct {
	Config *Config   // Configuration settings
	DB     *DB       // Database handler
	OAI    *OpenAI   // OpenAI handler
	TB     *Telegram // Telegram bot handler
}

// NewApp creates and initializes a new App instance.
func NewApp() (*App, error) {
	app := &App{}
	var err error

	// Initialize configuration
	app.Config, err = NewConfig()
	if err != nil {
		return nil, WrapError("failed to load config", err)
	}

	// Initialize database
	app.DB, err = NewDB(app.Config)
	if err != nil {
		return nil, WrapError("failed to init database", err)
	}

	// Initialize OpenAI
	app.OAI, err = NewOpenAI(app.Config)
	if err != nil {
		return nil, WrapError("failed to init OpenAI", err)
	}

	// Initialize Telegram bot
	app.TB, err = NewTelegram(app.Config, app.DB, app.OAI)
	if err != nil {
		return nil, WrapError("failed to init Telegram bot", err)
	}

	return app, nil
}

// Run starts the App and handles graceful shutdown.
func (app *App) Run() error {
	// Start the Telegram bot
	err := app.TB.Start()
	if err != nil {
		return WrapError("failed to start Telegram bot", err)
	}
	return nil
}

func main() {
	// Initialize the application
	app, err := NewApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize app")
	}

	// Run the application
	err = app.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start app")
	}
}
