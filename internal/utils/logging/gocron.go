package logging

import (
	"github.com/go-co-op/gocron/v2"
)

// GocronLogger implements gocron.Logger interface using our logging package.
type GocronLogger struct{}

// Interface return is required to satisfy gocron's Logger interface contract
//
//nolint:ireturn // Intentionally returning interface to implement gocron's logger system
func NewGocronLogger() gocron.Logger {
	return &GocronLogger{}
}

func (l *GocronLogger) Debug(msg string, args ...any) {
	Debug(msg, args...)
}

func (l *GocronLogger) Error(msg string, args ...any) {
	Error(msg, args...)
}

func (l *GocronLogger) Info(msg string, args ...any) {
	Info(msg, args...)
}

func (l *GocronLogger) Warn(msg string, args ...any) {
	Warn(msg, args...)
}
