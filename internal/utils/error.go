// Package utils provides common utility functions and patterns
package utils

import "fmt"

// Category represents error categories for better organization
type Category string

const (
	CategoryConfig     Category = "CONFIG"     // Configuration and initialization errors
	CategoryOperation  Category = "OPERATION"  // Runtime operation errors (timeouts, retries, etc.)
	CategoryExternal   Category = "EXTERNAL"   // External service errors (API, network, etc.)
	CategoryValidation Category = "VALIDATION" // Input/parameter validation errors
)

// Common error codes
const (
	ErrInvalidConfig = "INVALID_CONFIG"
	ErrOperation     = "OPERATION_ERROR"
	ErrTimeout       = "TIMEOUT_ERROR"
	ErrCircuitOpen   = "CIRCUIT_OPEN"
	ErrAPI           = "API_ERROR"
	ErrConnection    = "CONNECTION_ERROR"
	ErrValidation    = "VALIDATION_ERROR"
)

// AppError represents a standardized application error
type AppError struct {
	Code      string
	Message   string
	Component string
	Category  Category
	Err       error
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Component, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Component, e.Code, e.Message)
}

// Unwrap implements the errors unwrapping interface
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewError creates a new application error
func NewError(component string, code string, msg string, category Category, err error) error {
	return &AppError{
		Code:      code,
		Message:   msg,
		Component: component,
		Category:  category,
		Err:       err,
	}
}

// Errorf creates a new error with formatted message
func Errorf(component string, code string, category Category, format string, args ...interface{}) error {
	return &AppError{
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		Component: component,
		Category:  category,
	}
}
