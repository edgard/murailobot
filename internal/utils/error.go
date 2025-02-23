// Package utils provides common utility functions and patterns for error handling,
// logging, resilience, and text processing. It standardizes error reporting and
// categorization across the application.
package utils

import "fmt"

// Category represents error categories for better organization and handling.
// Categories help route errors to appropriate handling logic and provide
// meaningful grouping for monitoring and reporting.
type Category string

const (
	// CategoryConfig indicates errors related to configuration loading,
	// validation, or initialization of application components.
	CategoryConfig Category = "CONFIG"

	// CategoryOperation indicates runtime operational errors such as
	// timeouts, retries, or internal processing failures.
	CategoryOperation Category = "OPERATION"

	// CategoryExternal indicates errors from external service interactions
	// such as API calls, network operations, or third-party services.
	CategoryExternal Category = "EXTERNAL"

	// CategoryValidation indicates input validation errors, parameter
	// validation failures, or data consistency issues.
	CategoryValidation Category = "VALIDATION"
)

// Common error codes used throughout the application.
// These codes provide standardized error identification and
// should be used consistently across all components.
const (
	// ErrInvalidConfig indicates configuration validation or loading failures
	ErrInvalidConfig = "INVALID_CONFIG"

	// ErrOperation indicates general operational failures
	ErrOperation = "OPERATION_ERROR"

	// ErrTimeout indicates operation timeout errors
	ErrTimeout = "TIMEOUT_ERROR"

	// ErrCircuitOpen indicates circuit breaker is preventing operations
	ErrCircuitOpen = "CIRCUIT_OPEN"

	// ErrAPI indicates external API call failures
	ErrAPI = "API_ERROR"

	// ErrConnection indicates network or connection failures
	ErrConnection = "CONNECTION_ERROR"

	// ErrValidation indicates input validation failures
	ErrValidation = "VALIDATION_ERROR"
)

// AppError represents a standardized application error with context.
// It includes:
// - Component identification for error source tracking
// - Error code for programmatic error handling
// - Human-readable message for logging and display
// - Error category for routing and handling
// - Original error (if any) for debugging
type AppError struct {
	Code      string   // Unique identifier for the error type
	Message   string   // Human-readable error description
	Component string   // Source component where the error occurred
	Category  Category // Broad categorization of the error
	Err       error    // Original error if wrapping another error
}

// Error implements the error interface.
// It formats the error message to include all context in a consistent format:
// [Component] Code: Message: Underlying error (if any)
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Component, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Component, e.Code, e.Message)
}

// Unwrap implements the errors unwrapping interface.
// This allows AppError to work with errors.Is and errors.As functions
// for error inspection and handling.
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewError creates a new application error with the provided context.
// Example usage:
//
//	if err != nil {
//	    return NewError("db", ErrConnection, "failed to connect", CategoryExternal, err)
//	}
func NewError(component string, code string, msg string, category Category, err error) error {
	return &AppError{
		Code:      code,
		Message:   msg,
		Component: component,
		Category:  category,
		Err:       err,
	}
}

// Errorf creates a new error with a formatted message.
// It's similar to fmt.Errorf but includes application error context.
// Example usage:
//
//	return Errorf("config", ErrValidation, CategoryValidation,
//	    "invalid port number: %d", port)
func Errorf(component string, code string, category Category, format string, args ...interface{}) error {
	return &AppError{
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		Component: component,
		Category:  category,
	}
}
