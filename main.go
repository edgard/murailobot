package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/edgard/murailobot/internal/ai"
	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/logger"
	"github.com/edgard/murailobot/internal/telegram"
)

func main() {
	// Load configuration using the config package
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Setup(&cfg.Log); err != nil {
		slog.Error("failed to initialize logger", "error", err)
		os.Exit(1)
	}

	// Initialize database
	dbConfig := &db.Config{
		Name:            cfg.Database.Name,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		MaxMessageSize:  cfg.Database.MaxMessageSize,
	}
	database, err := db.New(dbConfig)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Initialize AI client
	aiConfig := &ai.Config{
		Token:       cfg.OpenAI.Token,
		BaseURL:     cfg.OpenAI.BaseURL,
		Model:       cfg.OpenAI.Model,
		Temperature: cfg.OpenAI.Temperature,
		TopP:        cfg.OpenAI.TopP,
		Instruction: cfg.OpenAI.Instruction,
		Timeout:     cfg.OpenAI.Timeout,
	}
	aiClient, err := ai.New(aiConfig, database)
	if err != nil {
		slog.Error("failed to initialize AI client", "error", err)
		os.Exit(1)
	}

	// Initialize bot
	tgConfig := &telegram.Config{
		Token:    cfg.Bot.Token,
		AdminUID: cfg.Bot.AdminUID,
		Messages: telegram.BotMessages{
			Welcome:        cfg.Bot.Messages.Welcome,
			NotAuthorized:  cfg.Bot.Messages.NotAuthorized,
			ProvideMessage: cfg.Bot.Messages.ProvideMessage,
			MessageTooLong: cfg.Bot.Messages.MessageTooLong,
			AIError:        cfg.Bot.Messages.AIError,
			GeneralError:   cfg.Bot.Messages.GeneralError,
			HistoryReset:   cfg.Bot.Messages.HistoryReset,
		},
		Polling: telegram.PollingConfig{
			Timeout:            cfg.Bot.PollTimeout,
			RequestTimeout:     cfg.Bot.RequestTimeout,
			MaxRoutines:        cfg.Bot.MaxRoutines,
			DropPendingUpdates: cfg.Bot.DropPendingUpdates,
		},
		TypingInterval:      cfg.Bot.TypingInterval,
		TypingActionTimeout: cfg.Bot.TypingActionTimeout,
		DBOperationTimeout:  cfg.Bot.DBOperationTimeout,
		AIRequestTimeout:    cfg.Bot.AIRequestTimeout,
	}
	securityConfig := &telegram.SecurityConfig{
		MaxMessageLength: cfg.Bot.MaxMessageLength,
		AllowedUserIDs:   cfg.Bot.AllowedUserIDs,
		BlockedUserIDs:   cfg.Bot.BlockedUserIDs,
	}
	bot, err := telegram.New(tgConfig, securityConfig, database, aiClient)
	if err != nil {
		slog.Error("failed to initialize bot", "error", err)
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
				slog.Info("received signal", "signal", sig)
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
		slog.Error("bot error", "error", err)
		os.Exit(1)
	}

	fmt.Println("Bot stopped")
}
