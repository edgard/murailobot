package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/telegram"
	"github.com/edgard/murailobot/internal/utils"
)

func main() {
	// Load configuration using the config package
	config, err := config.Load()
	if err != nil {
		utils.WriteErrorLog("main", "failed to load configuration", err,
			utils.KeyAction, "load_config")
		os.Exit(1)
	}

	// Initialize logger
	logCfg := &utils.LogConfig{
		Level:  config.Log.Level,
		Format: config.Log.Format,
	}
	if err := utils.Setup(logCfg); err != nil {
		utils.WriteErrorLog("main", "failed to initialize logger", err,
			utils.KeyAction, "init_logger")
		os.Exit(1)
	}

	// Initialize database
	database, err := db.New(config)
	if err != nil {
		utils.WriteErrorLog("main", "failed to initialize database", err,
			utils.KeyAction, "init_database")
		os.Exit(1)
	}
	defer database.Close()

	// Initialize AI client
	aiClient, err := ai.New(&config.AI, database)
	if err != nil {
		utils.WriteErrorLog("main", "failed to initialize AI client", err,
			utils.KeyAction, "init_ai")
		os.Exit(1)
	}

	// Initialize bot
	var bot telegram.BotService
	bot, err = telegram.New(config, database, aiClient)
	if err != nil {
		utils.WriteErrorLog("main", "failed to initialize bot", err,
			utils.KeyAction, "init_bot")
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer func() {
			signal.Stop(sigChan)
			close(sigChan)
		}()

		for {
			select {
			case sig, ok := <-sigChan:
				if !ok {
					return
				}
				utils.WriteInfoLog("main", "received shutdown signal",
					utils.KeyAction, "shutdown",
					utils.KeyReason, sig.String())
				cancel()
				return
			case <-ctx.Done():
				// Context was cancelled elsewhere
				return
			}
		}
	}()

	// Start bot
	if err := bot.Start(ctx); err != nil {
		utils.WriteErrorLog("main", "bot error", err,
			utils.KeyAction, "run_bot")
		os.Exit(1)
	}

	utils.WriteInfoLog("main", "bot stopped",
		utils.KeyAction, "shutdown",
		utils.KeyResult, "shutdown_complete")
}
