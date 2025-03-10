// Package logging provides structured logging configuration.
package logging

import "errors"

// log levels for internal use.
const (
	logLevelDebug = "debug"
	logLevelWarn  = "warn"
	logLevelError = "error"
	logLevelInfo  = "info"
)

// log formats for internal use.
const (
	logFormatText = "text"
	logFormatJSON = "json"
)

var (
	// ErrInvalidLogLevel indicates an invalid logging level was specified.
	ErrInvalidLogLevel = errors.New("invalid log level")
	// ErrInvalidLogFormat indicates an invalid logging format was specified.
	ErrInvalidLogFormat = errors.New("invalid log format")
)
