package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/edgard/murailobot/internal/config"
	"github.com/edgard/murailobot/internal/db"
	"github.com/edgard/murailobot/internal/openai"
	"github.com/edgard/murailobot/internal/telegram"
)

func main() {
	// Configure zerolog to output in a human-friendly format.
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load configuration from environment variables.
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize the database.
	database, err := db.NewDB(cfg.DBName)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize database")
	}

	// Initialize the OpenAI client.
	oaiClient, err := openai.NewClient(cfg.OpenAIToken, cfg.OpenAIInstruction, cfg.OpenAIModel, cfg.OpenAITemperature, cfg.OpenAITopP)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize OpenAI client")
	}

	// Initialize the Telegram bot.
	tgBot, err := telegram.NewBot(cfg, database, oaiClient)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize Telegram bot")
	}

	// Create a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for OS termination signals to gracefully shut down the bot.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("received termination signal, shutting down...")
		cancel()
		tgBot.Stop()
	}()

	// Start the Telegram bot; this call blocks until the bot stops.
	if err := tgBot.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Telegram bot error")
	}

	log.Info().Msg("bot stopped gracefully")
	time.Sleep(1 * time.Second)
}
