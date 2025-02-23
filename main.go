// Package main implements a Telegram bot with AI capabilities.
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

const componentName = "main"

func main() {
	config, err := config.Load()
	if err != nil {
		utils.ErrorLog(componentName, "failed to load configuration", err,
			utils.KeyAction, "load_config")
		os.Exit(1)
	}

	logCfg := &utils.LogConfig{
		Level:  config.Log.Level,
		Format: config.Log.Format,
	}
	if err := utils.Setup(logCfg); err != nil {
		utils.ErrorLog(componentName, "failed to initialize logger", err,
			utils.KeyAction, "init_logger")
		os.Exit(1)
	}

	database, err := db.New(config)
	if err != nil {
		utils.ErrorLog(componentName, "failed to initialize database", err,
			utils.KeyAction, "init_database")
		os.Exit(1)
	}
	defer database.Close()

	aiClient, err := ai.New(&config.AI, database)
	if err != nil {
		utils.ErrorLog(componentName, "failed to initialize AI client", err,
			utils.KeyAction, "init_ai")
		os.Exit(1)
	}

	var bot telegram.BotService
	bot, err = telegram.New(config, database, aiClient)
	if err != nil {
		utils.ErrorLog(componentName, "failed to initialize bot", err,
			utils.KeyAction, "init_bot")
		os.Exit(1)
	}

	// Handle SIGINT/SIGTERM for graceful shutdown
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
				utils.InfoLog(componentName, "received shutdown signal",
					utils.KeyAction, "shutdown",
					utils.KeyReason, sig.String())
				cancel()
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := bot.Start(ctx); err != nil {
		utils.ErrorLog(componentName, "bot error", err,
			utils.KeyAction, "run_bot")
		os.Exit(1)
	}

	utils.InfoLog(componentName, "bot stopped",
		utils.KeyAction, "shutdown",
		utils.KeyResult, "shutdown_complete")
}
