// Package utils provides common utility functions and patterns.
// This file implements structured logging with consistent formatting,
// attribute handling, and log level management using slog.
package utils

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Core logger constants define size limits and component identification.
const (
	componentName = "logger"
	maxLogSize    = 32 * 1024 // 32KB limit for individual log messages
)

// Standard log keys for structured logging provide a consistent vocabulary
// for log attributes across the application. These keys should be used
// instead of ad-hoc attribute names to ensure logs are searchable and
// maintain a standard format.
const (
	// Core attributes identify the source and context of log entries
	KeyComponent = "component" // Component/module name (required in all logs)
	KeyError     = "error"     // Error details for error logging
	KeyUserID    = "user_id"   // User identifier for user-related operations
	KeyResult    = "result"    // Operation result (success/failure/etc.)
	KeyReason    = "reason"    // Reason for an action or state change

	// Metrics and measurements provide quantitative context
	KeyLimit     = "limit"      // Limit values (e.g., page size, rate limits)
	KeyCount     = "count"      // Count of items (e.g., results, attempts)
	KeySize      = "size"       // Size of data (e.g., message length, file size)
	KeyRequestID = "request_id" // Request identifier for operation tracking
	KeyRequested = "requested"  // Originally requested value

	// State and transitions track operational flow
	KeyFrom   = "from_state" // Previous state in state transitions
	KeyTo     = "to_state"   // New state in state transitions
	KeyAction = "action"     // Action being performed

	// Resource information identifies affected entities
	KeyName = "name" // Resource name or identifier
	KeyType = "type" // Resource or operation type

	// Transaction information for database operations
	KeyTxType = "transaction" // Transaction type or category
)

// LogConfig defines logging configuration parameters.
// Both fields are validated using struct tags to ensure
// only supported values are used.
type LogConfig struct {
	Level  string `validate:"required,oneof=debug info warn error"`
	Format string `validate:"required,oneof=json text"`
}

var (
	// Default logger settings provide basic logging capability
	// before Setup() is called. These ensure logging is always
	// available, even during initialization.
	defaultLevel  = slog.LevelInfo
	defaultFormat = "json"

	// Logger initialization state is protected by a mutex
	// to ensure thread-safe setup and access.
	loggerMu sync.RWMutex
	isSetup  bool
)

// levelMap provides direct mapping from configuration strings
// to slog.Level values. This map is used during setup to
// convert the configured level string to a slog.Level.
var levelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// truncateString ensures a string doesn't exceed maxLogSize.
// It adds an ellipsis (...) when truncation occurs to indicate
// the string was cut off.
func truncateString(s string) string {
	if len(s) <= maxLogSize {
		return s
	}
	return s[:maxLogSize-3] + "..."
}

// customHandler wraps a slog.Handler to provide additional functionality:
// - Message and attribute size limiting
// - Consistent time formatting
// - Source location tracking
type customHandler struct {
	handler slog.Handler
}

// Enabled implements slog.Handler interface.
// It delegates the enabled check to the wrapped handler.
func (h *customHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle implements slog.Handler interface.
// It processes the log record before passing it to the wrapped handler:
// - Truncates oversized messages
// - Truncates oversized attribute values
// - Ensures consistent attribute formatting
func (h *customHandler) Handle(ctx context.Context, r slog.Record) error {
	if len(r.Message) > maxLogSize {
		r = slog.NewRecord(r.Time, r.Level, truncateString(r.Message), r.PC)
	}

	r.Attrs(func(attr slog.Attr) bool {
		if s := attr.Value.String(); len(s) > maxLogSize {
			r.Add(attr.Key, truncateString(s))
		}
		return true
	})

	return h.handler.Handle(ctx, r)
}

// WithAttrs implements slog.Handler interface.
// It wraps the new handler to maintain custom handling.
func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{handler: h.handler.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler interface.
// It wraps the new handler to maintain custom handling.
func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{handler: h.handler.WithGroup(name)}
}

// Setup initializes the logger with the given configuration.
// It configures:
// - Log level (debug, info, warn, error)
// - Output format (json, text)
// - Custom attribute handling
// - UTC timestamp formatting
// This function is thread-safe and can be called multiple times,
// but typically should only be called once during application startup.
func Setup(cfg *LogConfig) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if cfg == nil {
		return NewError(componentName, ErrValidation, "configuration is nil", CategoryValidation, nil)
	}

	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	cfg.Format = strings.ToLower(strings.TrimSpace(cfg.Format))

	level, ok := levelMap[cfg.Level]
	if !ok {
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

// getDefaultHandler returns a handler with default settings.
// This is used when logging is needed before Setup() is called.
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

// writeLog is a generic logging function that ensures consistent formatting
// and attribute handling across all log levels. It automatically adds
// the component name as a required attribute.
func writeLog(level slog.Level, component, msg string, attrs ...any) {
	ensureLogger()

	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyComponent, component)
	args = append(args, attrs...)

	slog.Log(context.Background(), level, msg, args...)
}

// WriteErrorLog logs an error with consistent attributes.
// It automatically includes the error value and any additional
// attributes provided.
func WriteErrorLog(component string, msg string, err error, attrs ...any) {
	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyError, err)
	args = append(args, attrs...)
	writeLog(slog.LevelError, component, msg, args...)
}

// WriteWarnLog logs a warning with consistent attributes.
// Use this for potentially problematic situations that don't
// prevent normal operation.
func WriteWarnLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelWarn, component, msg, attrs...)
}

// WriteInfoLog logs an info message with consistent attributes.
// Use this for normal operational events that highlight the
// progress of the application.
func WriteInfoLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelInfo, component, msg, attrs...)
}

// WriteDebugLog logs a debug message with consistent attributes.
// Use this for detailed information useful during debugging
// and development.
func WriteDebugLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelDebug, component, msg, attrs...)
}
