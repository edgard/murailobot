package logging

import (
	"github.com/go-co-op/gocron/v2"
)

// gocronLogger implements gocron.Logger interface using our logging package.
type gocronLogger struct{}

// NewGocronLogger returns a new logger that implements gocron.Logger interface.
//
//nolint:ireturn // Interface return is required by gocron's API contract
func NewGocronLogger() gocron.Logger {
	return &gocronLogger{}
}

func (l *gocronLogger) Debug(msg string, args ...any) {
	Debug(msg, args...)
}

func (l *gocronLogger) Error(msg string, args ...any) {
	Error(msg, args...)
}

func (l *gocronLogger) Info(msg string, args ...any) {
	Info(msg, args...)
}

func (l *gocronLogger) Warn(msg string, args ...any) {
	Warn(msg, args...)
}
