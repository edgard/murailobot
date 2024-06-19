package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

// App encapsulates the entire application.
type App struct {
	Logger *zap.Logger
	Config *Config
	DB     *DB
	OAI    *OpenAI
	TB     *Telegram
}

// NewApp creates and initializes a new App instance.
func NewApp() (*App, error) {
	var err error
	app := &App{}

	app.Logger, err = initLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %v", err)
	}

	app.Config, err = NewConfig()
	if err != nil {
		app.Logger.Error("Failed to load config", zap.Error(err))
		return nil, err
	}

	app.DB, err = NewDB("storage.db")
	if err != nil {
		app.Logger.Error("Failed to init database", zap.Error(err))
		return nil, err
	}

	app.OAI = NewOpenAI(app.Config.OpenAIToken, app.Config.OpenAIInstruction)
	if err := app.OAI.Ping(); err != nil {
		app.Logger.Error("Failed to connect to OpenAI", zap.Error(err))
		return nil, err
	}

	app.TB, err = NewTelegram(app.Config.TelegramToken, app.DB, app.OAI, app.Config, app.Logger)
	if err != nil {
		app.Logger.Error("Failed to init telegram bot", zap.Error(err))
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
	app.Logger.Info("Shutting down")
	app.Logger.Sync()
	app.DB.conn.Close()
}

func initLogger() (*zap.Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	return logger, nil
}

func main() {
	app, err := NewApp()
	if err != nil {
		panic(err)
	}
	app.Run()
}
