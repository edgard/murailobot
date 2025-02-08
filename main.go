package main

import (
	"github.com/rs/zerolog/log"
)

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize app")
	}
	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to run app")
	}
}
