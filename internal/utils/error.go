// Package utils provides error handling, logging, and resilience patterns.
package utils

import "fmt"

type Category string

const (
	CategoryConfig     Category = "CONFIG"     // Configuration and initialization
	CategoryOperation  Category = "OPERATION"  // Runtime operations
	CategoryExternal   Category = "EXTERNAL"   // External services
	CategoryValidation Category = "VALIDATION" // Input validation
)

const (
	ErrInvalidConfig = "INVALID_CONFIG"
	ErrOperation     = "OPERATION_ERROR"
	ErrTimeout       = "TIMEOUT_ERROR"
	ErrCircuitOpen   = "CIRCUIT_OPEN"
	ErrAPI           = "API_ERROR"
	ErrConnection    = "CONNECTION_ERROR"
	ErrValidation    = "VALIDATION_ERROR"
)

// AppError provides standardized error handling with source tracking
type AppError struct {
	Code      string
	Message   string
	Component string
	Category  Category
	Err       error // Original error if wrapping
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Component, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Component, e.Code, e.Message)
}

// Unwrap implements errors.Unwrap interface
func (e *AppError) Unwrap() error {
	return e.Err
}

// Example:
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

// Example:
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
