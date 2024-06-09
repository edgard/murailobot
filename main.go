package main

import (
	"fmt"

	"go.uber.org/zap"
)

var logger *zap.Logger

func main() {
	initLogger()
	defer logger.Sync()

	if err := loadConfig(); err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	if err := initDatabase(); err != nil {
		logger.Fatal("Failed to init database", zap.Error(err))
	}
	if err := initTelegramBot(); err != nil {
		logger.Fatal("Failed to init telegram bot", zap.Error(err))
	}

	startHttpServer()
	startTelegramBot()
}

func initLogger() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
}
