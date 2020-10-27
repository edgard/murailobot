package main

import "time"

type Configuration struct {
	Token string
	DB    string
}

type MessageReference struct {
	MessageID int   `db:"message_id"`
	ChatID    int64 `db:"chat_id"`
}

type User struct {
	UserID	int
	Timestamp time.Time
}
