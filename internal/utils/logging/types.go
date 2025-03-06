// Package logging provides structured logging configuration.
package logging

import "errors"

// Log levels.
const (
	LogLevelDebug = "debug"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
	LogLevelInfo  = "info"
)

// Log formats.
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

var (
	ErrInvalidLogLevel  = errors.New("invalid log level")
	ErrInvalidLogFormat = errors.New("invalid log format")
)
