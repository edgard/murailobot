package logging

import (
	"errors"
	"fmt"
	"strings"

	"github.com/edgard/murailobot/internal/errs"
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
	processedArgs := processSchedulerArgs(msg, args...)
	Debug(msg, processedArgs...)
}

func (l *gocronLogger) Error(msg string, args ...any) {
	processedArgs := processSchedulerArgs(msg, args...)
	Error(msg, processedArgs...)
}

func (l *gocronLogger) Info(msg string, args ...any) {
	processedArgs := processSchedulerArgs(msg, args...)
	Info(msg, processedArgs...)
}

func (l *gocronLogger) Warn(msg string, args ...any) {
	processedArgs := processSchedulerArgs(msg, args...)
	Warn(msg, processedArgs...)
}

// processSchedulerArgs enhances scheduler log messages with better error handling and context.
func processSchedulerArgs(msg string, args ...any) []any {
	// Convert args to a slice we can modify
	processedArgs := make([]any, 0, len(args))

	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			// If we have an odd number of args, append the last one as is
			processedArgs = append(processedArgs, args[i])

			break
		}

		key, val := args[i], args[i+1]

		// Special handling for error values
		if key == "error" || fmt.Sprint(key) == "error" {
			if err, ok := val.(error); ok {
				// Categorize scheduler errors
				var wrappedErr error

				switch {
				case errors.Is(err, gocron.ErrJobNotFound):
					wrappedErr = errs.NewValidationError("scheduled job not found", err)
				case strings.Contains(err.Error(), "duplicate job"):
					wrappedErr = errs.NewValidationError("duplicate job name", err)
				case strings.Contains(err.Error(), "job does not exist"):
					wrappedErr = errs.NewValidationError("job does not exist", err)
				case strings.Contains(err.Error(), "job is disabled"):
					wrappedErr = errs.NewConfigError("job is disabled", err)
				case strings.Contains(err.Error(), "no jobs"):
					wrappedErr = errs.NewConfigError("no jobs scheduled", err)
				case strings.Contains(err.Error(), "shutdown"):
					wrappedErr = errs.NewConfigError("scheduler is shut down", err)
				default:
					// For unknown scheduler errors, wrap as config error
					wrappedErr = errs.NewConfigError("scheduler error", err)
				}

				processedArgs = append(processedArgs, key, wrappedErr)

				continue
			}
		}

		// For non-error key-value pairs, add them as is
		processedArgs = append(processedArgs, key, val)
	}

	return processedArgs
}
