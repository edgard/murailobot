package main

// App encapsulates the entire application.
type App struct {
	Config *Config   // Configuration settings.
	DB     *DB       // Database handler.
	OAI    *OpenAI   // OpenAI client.
	TB     *Telegram // Telegram bot handler.
}

// NewApp initializes the application by creating the config, DB, OpenAI, and Telegram instances.
func NewApp() (*App, error) {
	app := &App{}
	var err error

	app.Config, err = NewConfig()
	if err != nil {
		return nil, WrapError("failed to load config", err)
	}

	app.DB, err = NewDB(app.Config)
	if err != nil {
		return nil, WrapError("failed to init database", err)
	}

	app.OAI, err = NewOpenAI(app.Config)
	if err != nil {
		return nil, WrapError("failed to init OpenAI", err)
	}

	app.TB, err = NewTelegram(app.Config, app.DB, app.OAI)
	if err != nil {
		return nil, WrapError("failed to init Telegram bot", err)
	}

	return app, nil
}

// Run starts the Telegram bot (and potentially other components).
func (app *App) Run() error {
	if err := app.TB.Start(); err != nil {
		return WrapError("failed to start Telegram bot", err)
	}
	return nil
}
