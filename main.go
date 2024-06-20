package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
)

// App encapsulates the entire application.
type App struct {
	Config *Config
	DB     *DB
	OAI    *OpenAI
	TB     *Telegram
}

// NewApp creates and initializes a new App instance.
func NewApp() (*App, error) {
	var err error
	app := &App{}

	app.Config, err = NewConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		return nil, err
	}

	app.DB, err = NewDB(app.Config)
	if err != nil {
		log.Error().Err(err).Msg("Failed to init database")
		return nil, err
	}

	app.OAI = NewOpenAI(app.Config)
	if err := app.OAI.Ping(); err != nil {
		log.Error().Err(err).Msg("Failed to connect to OpenAI")
		return nil, err
	}

	app.TB, err = NewTelegram(app.Config, app.DB, app.OAI)
	if err != nil {
		log.Error().Err(err).Msg("Failed to init telegram bot")
		return nil, err
	}

	return app, nil
}

// Run starts the App and handles graceful shutdown.
func (app *App) Run() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go app.TB.Start()

	<-stop
	log.Info().Msg("Shutting down")
	app.DB.conn.Close()
}

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize app")
	}
	app.Run()
}
