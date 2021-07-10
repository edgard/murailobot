package main

import (
	"errors"
	"time"

	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	log "github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var config Config

func sendMessagePiu(db *gorm.DB, bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	var user User
	if err := db.Where("user_id = ?", update.Message.From.ID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.WithFields(log.Fields{
				"user_id":  update.Message.From.ID,
				"username": update.Message.From.UserName,
			}).Info("User not found, creating new user")
			user = User{UserID: update.Message.From.ID, LastUsed: time.Now().Add(-time.Minute * time.Duration(config.UserTimeout+1))}
			if err := db.Create(&user).Error; err != nil {
				log.WithError(err).Info("Error creating new user")
			}
		} else {
			log.WithError(err).Info("Error querying for user")
			return
		}
	}

	if time.Since(user.LastUsed).Minutes() <= config.UserTimeout {
		log.WithFields(log.Fields{
			"user_id":   user.UserID,
			"username":  update.Message.From.UserName,
			"last_used": int(time.Since(user.LastUsed).Minutes()),
		}).Info("User on timeout")
		return
	}

	user.LastUsed = time.Now()
	if err := db.Save(&user).Error; err != nil {
		log.WithError(err).Info("Error updating user last_updated timestamp")
	}

	var msgRef MessageRef
	result := db.Where("id IN (?)", db.Order("last_used ASC").Limit(5).Select("id").Table("message_refs")).Order("RANDOM()").Take(&msgRef)
	if result.Error != nil {
		log.WithError(result.Error).Info("Failed getting message reference")
		return
	}

	msgRef.LastUsed = time.Now()
	if err := db.Save(&msgRef).Error; err != nil {
		log.WithError(err).Info("Error updating message reference last_updated timestamp")
	}

	log.WithFields(log.Fields{
		"msgref_chatid":   msgRef.ChatID,
		"msgref_id":       msgRef.MessageID,
		"update_user_id":  update.Message.From.ID,
		"update_username": update.Message.From.UserName,
	}).Info("Sending forward")
	sendForward(bot, update, msgRef.ChatID, msgRef.MessageID)
}

func sendMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, text string) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.WithError(err).Warn("Failed to send message")
	}
}

func sendForward(bot *tgbotapi.BotAPI, update tgbotapi.Update, forwardChatID int64, forwardMessageID int) {
	msg := tgbotapi.NewForward(update.Message.Chat.ID, forwardChatID, forwardMessageID)
	if _, err := bot.Send(msg); err != nil {
		log.WithError(err).Warn("Failed to send forward")
	}
}

func processMessage(db *gorm.DB, bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	if update.Message.ForwardDate == 0 {
		log.WithField("update_id", update.UpdateID).Info("Ignoring non-forward update")
		return
	}

	msgRef := MessageRef{MessageID: update.Message.MessageID, ChatID: update.Message.Chat.ID, LastUsed: time.Now()}
	if err := db.Create(&msgRef).Error; err != nil {
		log.WithError(err).Info("Error creating new message reference")
		return
	}

	log.WithField("id", msgRef.ID).Info("New message reference created")
	sendMessage(bot, update, "Mensagem adicionada ao banco de dados!")
}

func main() {
	// init config
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.WithError(err).Fatal("Error reading config file")
		}
	}

	// defaults
	viper.SetDefault("update_timeout", 60)
	viper.SetDefault("user_timeout", 5)
	viper.SetDefault("telegram_debug", false)
	viper.SetDefault("db_name", "storage.db")

	if err := viper.Unmarshal(&config); err != nil {
		log.WithError(err).Fatal("Unable to decode config file")
	}

	// init db
	db, err := gorm.Open(sqlite.Open(config.DBName), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}

	// migrate the schema
	if err := db.AutoMigrate(&MessageRef{}, &User{}); err != nil {
		log.WithError(err).Fatal("Database auto migration failed")
	}

	// start bot
	log.Info("Logging in to Telegram")
	bot, err := tgbotapi.NewBotAPI(config.AuthToken)
	if err != nil {
		log.WithError(err).Fatal("Error logging in to Telegram")
	}
	bot.Debug = config.TelegramDebug

	log.WithField("username", bot.Self.UserName).Info("Logged in to Telegram")

	// messages loop
	u := tgbotapi.NewUpdate(0)
	u.Timeout = config.UpdateTimeout

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.WithError(err).Warn("Failed to get updates")
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if !update.Message.IsCommand() {
			processMessage(db, bot, update)
			continue
		}

		switch update.Message.Command() {
		case "piu":
			sendMessagePiu(db, bot, update)
		case "start":
			sendMessage(bot, update, "Ola! Me faça forward de uma mensagem para guardar.")
		}
	}
}
