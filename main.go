package main

import (
	"fmt"

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
	if app.Config, err = NewConfig(); err != nil {
		return nil, WrapError(fmt.Errorf("failed to load config: %w", err))
	}

	// Initialize database
	if app.DB, err = NewDB(app.Config); err != nil {
		return nil, WrapError(fmt.Errorf("failed to init database: %w", err))
	}

	// Initialize OpenAI
	if app.OAI, err = NewOpenAI(app.Config); err != nil {
		return nil, WrapError(fmt.Errorf("failed to init OpenAI: %w", err))
	}

	// Initialize Telegram bot
	if app.TB, err = NewTelegram(app.Config, app.DB, app.OAI); err != nil {
		return nil, WrapError(fmt.Errorf("failed to init Telegram bot: %w", err))
	}

	return app, nil
}

// Run starts the App and handles graceful shutdown.
func (app *App) Run() error {
	// Start the Telegram bot
	if err := app.TB.Start(); err != nil {
		return WrapError(fmt.Errorf("failed to start Telegram bot: %w", err))
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
	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start app")
	}
}
