package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/edgard/murailobot/internal/config"
	"github.com/go-playground/validator/v10"
)

// Custom handler to add source location and standardize time format
type customHandler struct {
	handler slog.Handler
}

func (h *customHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

const maxLogSize = 32 * 1024 // 32KB limit for log messages

func (h *customHandler) Handle(ctx context.Context, r slog.Record) error {
	// Limit message size
	msg := r.Message
	if len(msg) > maxLogSize {
		msg = msg[:maxLogSize-3] + "..."
		r = slog.NewRecord(r.Time, r.Level, msg, r.PC)

		// Create a new slice to store truncated attributes
		var attrs []slog.Attr
		r.Attrs(func(attr slog.Attr) bool {
			val := attr.Value.String()
			if len(val) > maxLogSize {
				attr.Value = slog.StringValue(val[:maxLogSize-3] + "...")
			}
			attrs = append(attrs, attr)
			return true
		})

		// Add truncated attributes back to the record
		for _, attr := range attrs {
			r.Add(attr.Key, attr.Value)
		}
	}

	return h.handler.Handle(ctx, r)
}

func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{handler: h.handler.WithGroup(name)}
}

// Setup initializes the logger with the given configuration
func Setup(cfg *config.LogConfig) error {
	if cfg == nil {
		return fmt.Errorf("logger configuration is nil")
	}

	// Normalize values before validation
	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	cfg.Format = strings.ToLower(strings.TrimSpace(cfg.Format))

	// Validate using struct tags
	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var errMsgs []string
			for _, e := range validationErrors {
				errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", e.Field(), e.Tag()))
			}
			return fmt.Errorf("invalid log configuration: %v", errMsgs)
		}
		return fmt.Errorf("log validation failed: %v", err)
	}

	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Standardize time format
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

	// Wrap with custom handler for additional functionality
	handler := &customHandler{handler: baseHandler}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("logger initialized",
		"level", cfg.Level,
		"format", cfg.Format,
		"time_format", "UTC",
	)

	return nil
}
