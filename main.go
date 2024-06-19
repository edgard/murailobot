package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

var (
	logger *zap.Logger
	config *Config
	db     *DB
	oai    *OpenAI
	tb     *TelegramBot
)

func main() {
	var err error

	logger, err = initLogger()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	config, err = initConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	db, err = initDB("storage.db")
	if err != nil {
		logger.Fatal("Failed to init database", zap.Error(err))
	}

	oai, err = initOpenAI()
	if err != nil {
		logger.Fatal("Failed to connect to OpenAI", zap.Error(err))
	}

	tb, err = initTelegramBot()
	if err != nil {
		logger.Fatal("Failed to init telegram bot", zap.Error(err))
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go tb.Start()

	<-stop
	logger.Info("Shutting down")
	logger.Sync()
	db.conn.Close()
}

func initLogger() (*zap.Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	return logger, nil
}

func initConfig() (*Config, error) {
	config := &Config{}
	if err := config.Init(); err != nil {
		return nil, err
	}
	return config, nil
}

func initDB(databasePath string) (*DB, error) {
	db := &DB{}
	if err := db.Init(databasePath); err != nil {
		return nil, err
	}
	return db, nil
}

func initOpenAI() (*OpenAI, error) {
	oai := &OpenAI{}
	if err := oai.Init(); err != nil {
		return nil, err
	}
	return oai, nil
}

func initTelegramBot() (*TelegramBot, error) {
	tb := &TelegramBot{}
	if err := tb.Init(); err != nil {
		return nil, err
	}
	return tb, nil
}
