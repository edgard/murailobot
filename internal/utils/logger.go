// Package utils provides structured logging with consistent formatting.
package utils

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	componentName = "logger"
	maxLogSize    = 32 * 1024 // 32KB limit
)

// Standard log keys for consistent attribute naming
const (
	// Core attributes
	KeyComponent = "component" // Required in all logs
	KeyError     = "error"
	KeyUserID    = "user_id"
	KeyResult    = "result"
	KeyReason    = "reason"

	// Metrics
	KeyLimit     = "limit"
	KeyCount     = "count"
	KeySize      = "size"
	KeyRequestID = "request_id"
	KeyRequested = "requested"

	// State
	KeyFrom   = "from_state"
	KeyTo     = "to_state"
	KeyAction = "action"

	// Resources
	KeyName = "name"
	KeyType = "type"

	// Transactions
	KeyTxType = "transaction"
)

type LogConfig struct {
	Level  string `validate:"required,oneof=debug info warn error"`
	Format string `validate:"required,oneof=json text"`
}

var (
	defaultLevel  = slog.LevelInfo
	defaultFormat = "json"

	loggerMu sync.RWMutex
	isSetup  bool
)

var levelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

func truncateString(s string) string {
	if len(s) <= maxLogSize {
		return s
	}
	return s[:maxLogSize-3] + "..."
}

type customHandler struct {
	handler slog.Handler
}

func (h *customHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

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

func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{handler: h.handler.WithGroup(name)}
}

// Setup initializes thread-safe logging with size limits and UTC timestamps
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

// getDefaultHandler provides basic logging before Setup
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

func ensureLogger() {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if !isSetup {
		logger := slog.New(&customHandler{handler: getDefaultHandler()})
		slog.SetDefault(logger)
		isSetup = true
	}
}

func writeLog(level slog.Level, component, msg string, attrs ...any) {
	ensureLogger()

	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyComponent, component)
	args = append(args, attrs...)

	slog.Log(context.Background(), level, msg, args...)
}

func ErrorLog(component string, msg string, err error, attrs ...any) {
	args := make([]any, 0, len(attrs)+2)
	args = append(args, KeyError, err)
	args = append(args, attrs...)
	writeLog(slog.LevelError, component, msg, args...)
}

func WarnLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelWarn, component, msg, attrs...)
}

func InfoLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelInfo, component, msg, attrs...)
}

func DebugLog(component string, msg string, attrs ...any) {
	writeLog(slog.LevelDebug, component, msg, attrs...)
}
