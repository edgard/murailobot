// Package utils provides common utility functions and patterns
package utils

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Core logger constants
const (
	componentName = "logger"
	maxLogSize    = 32 * 1024 // 32KB limit for log messages
)

// Standard log keys for structured logging.
// These keys are used consistently across the application to ensure
// logs are searchable, filterable, and maintain a standard format.
const (
	// Core attributes
	KeyComponent = "component" // Component/module name
	KeyError     = "error"     // Error details
	KeyUserID    = "user_id"   // User identifier
	KeyResult    = "result"    // Operation result
	KeyReason    = "reason"    // Reason for an action/state

	// Metrics and measurements
	KeyLimit     = "limit"      // Limit value
	KeyCount     = "count"      // Count of items
	KeySize      = "size"       // Size of data
	KeyRequestID = "request_id" // Request identifier (e.g., message_id, update_id)
	KeyRequested = "requested"  // Requested value (e.g., requested limit)

	// State and transitions
	KeyFrom   = "from_state" // Previous state
	KeyTo     = "to_state"   // New state
	KeyAction = "action"     // Action being performed

	// Resource information
	KeyName = "name" // Resource name
	KeyType = "type" // Type information

	// Transaction information
	KeyTxType = "transaction" // Transaction type
)

// LogConfig defines logging configuration
type LogConfig struct {
	Level  string `validate:"required,oneof=debug info warn error"`
	Format string `validate:"required,oneof=json text"`
}

var (
	// Default logger settings used as fallback when Setup() hasn't been called.
	// These defaults ensure basic logging capability is available even before
	// proper configuration, but will be overridden when Setup() is called.
	defaultLevel  = slog.LevelInfo
	defaultFormat = "json"

	// Logger initialization lock and state
	loggerMu sync.RWMutex
	isSetup  bool
)

// levelMap provides a direct mapping from string to slog.Level
var levelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// truncateString ensures a string doesn't exceed maxLogSize
func truncateString(s string) string {
	if len(s) <= maxLogSize {
		return s
	}
	return s[:maxLogSize-3] + "..."
}

// Custom handler to add source location and standardize time format
type customHandler struct {
	handler slog.Handler
}

func (h *customHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *customHandler) Handle(ctx context.Context, r slog.Record) error {
	// Truncate message if needed
	if len(r.Message) > maxLogSize {
		r = slog.NewRecord(r.Time, r.Level, truncateString(r.Message), r.PC)
	}

	// Process attributes efficiently
	r.Attrs(func(attr slog.Attr) bool {
		if s := attr.Value.String(); len(s) > maxLogSize {
			r.Add(attr.Key, truncateString(s))
		}
		return true
	})

	return h.handler.Handle(ctx, r)
}

func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{handler: h.handler.WithGroup(name)}
}

// Setup initializes the logger with the given configuration
func Setup(cfg *LogConfig) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if cfg == nil {
		return NewError(componentName, ErrValidation, "configuration is nil", CategoryValidation, nil)
	}

	// Normalize config values
	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	cfg.Format = strings.ToLower(strings.TrimSpace(cfg.Format))

	// Get log level from map (validation already done by struct tags)
	level, ok := levelMap[cfg.Level]
	if !ok {
		// This should never happen due to struct validation, but we handle it just in case
		return Errorf(componentName, ErrValidation, CategoryValidation,
			"unexpected log level: %s", cfg.Level)
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.Time(a.Key, t.UTC().Truncate(time.Millisecond))
				}
			}
			return a
		},
	}

	var baseHandler slog.Handler
	if cfg.Format == "text" {
		baseHandler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		baseHandler = slog.NewJSONHandler(os.Stderr, opts)
	}

	logger := slog.New(&customHandler{handler: baseHandler})
	slog.SetDefault(logger)
	isSetup = true

	slog.Info("logger initialized",
		KeyComponent, componentName,
		"level", cfg.Level,
		"format", cfg.Format,
	)

	return nil
}

// getDefaultHandler returns a handler with default settings
func getDefaultHandler() slog.Handler {
	opts := &slog.HandlerOptions{
		Level: defaultLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.Time(a.Key, t.UTC().Truncate(time.Millisecond))
				}
			}
			return a
		},
	}

	if defaultFormat == "text" {
		return slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.NewJSONHandler(os.Stderr, opts)
}

// ensureLogger ensures a logger is available by setting up a default logger
// if none has been configured. This provides basic logging capability
// before Setup() is called.
func ensureLogger() {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if !isSetup {
		logger := slog.New(&customHandler{handler: getDefaultHandler()})
		slog.SetDefault(logger)
		isSetup = true
	}
}

// writeLog is a generic logging function to reduce code duplication
func writeLog(level slog.Level, component, msg string, attrs ...any) {
	ensureLogger()

	// Pre-allocate slice with exact capacity needed
	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyComponent, component)
	args = append(args, attrs...)

	// Use the default logger directly
	slog.Log(context.Background(), level, msg, args...)
}

// WriteErrorLog logs an error with consistent attributes
func WriteErrorLog(component string, msg string, err error, attrs ...any) {
	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyError, err)
	args = append(args, attrs...)
	writeLog(slog.LevelError, component, msg, args...)
}

// WriteWarnLog logs a warning with consistent attributes
func WriteWarnLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelWarn, component, msg, attrs...)
}

// WriteInfoLog logs an info message with consistent attributes
func WriteInfoLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelInfo, component, msg, attrs...)
}

// WriteDebugLog logs a debug message with consistent attributes
func WriteDebugLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelDebug, component, msg, attrs...)
}
