// Package errors provides structured error types for the application with error codes
// that can be used for error handling and reporting.
package errors

import (
	"errors"
	"fmt"
)

// Error code constants for categorizing application errors.
const (
	CodeUnknown      = "UNKNOWN"
	CodeDatabase     = "DATABASE"
	CodeValidation   = "VALIDATION"
	CodeAPI          = "API"
	CodeConfig       = "CONFIG"
	CodeUnauthorized = "UNAUTHORIZED"
)

// ApplicationError is the interface that all custom application errors implement.
// It extends the standard error interface with methods to retrieve error codes
// and unwrap nested errors.
type ApplicationError interface {
	error
	Code() string
	Unwrap() error
}

// Error represents a basic application error with code, message and wrapped error.
type Error struct {
	code    string
	message string
	err     error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.message, e.err)
	}

	return e.message
}

func (e *Error) Code() string {
	return e.code
}

func (e *Error) Unwrap() error {
	return e.err
}

// Code returns the error code of an application error
// or CodeUnknown if it doesn't implement ApplicationError.
func Code(err error) string {
	var appErr ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Code()
	}

	return CodeUnknown
}

// DatabaseError represents database-related errors.
type DatabaseError struct {
	base Error
}

func (e *DatabaseError) Error() string {
	return e.base.Error()
}

func (e *DatabaseError) Code() string {
	return e.base.Code()
}

func (e *DatabaseError) Unwrap() error {
	return e.base.Unwrap()
}

// NewDatabaseError creates a new error for database operations.
func NewDatabaseError(message string, cause error) error {
	return &DatabaseError{
		base: Error{
			code:    CodeDatabase,
			message: message,
			err:     cause,
		},
	}
}

// ValidationError represents input validation errors.
type ValidationError struct {
	base Error
}

func (e *ValidationError) Error() string {
	return e.base.Error()
}

func (e *ValidationError) Code() string {
	return e.base.Code()
}

func (e *ValidationError) Unwrap() error {
	return e.base.Unwrap()
}

// NewValidationError creates a new error for validation failures.
func NewValidationError(message string, cause error) error {
	return &ValidationError{
		base: Error{
			code:    CodeValidation,
			message: message,
			err:     cause,
		},
	}
}

// APIError represents errors from external API calls.
type APIError struct {
	base Error
}

func (e *APIError) Error() string {
	return e.base.Error()
}

func (e *APIError) Code() string {
	return e.base.Code()
}

func (e *APIError) Unwrap() error {
	return e.base.Unwrap()
}

// NewAPIError creates a new error for API operation failures.
func NewAPIError(message string, cause error) error {
	return &APIError{
		base: Error{
			code:    CodeAPI,
			message: message,
			err:     cause,
		},
	}
}

// ConfigError represents configuration-related errors.
type ConfigError struct {
	base Error
}

func (e *ConfigError) Error() string {
	return e.base.Error()
}

func (e *ConfigError) Code() string {
	return e.base.Code()
}

func (e *ConfigError) Unwrap() error {
	return e.base.Unwrap()
}

// NewConfigError creates a new error for configuration issues.
func NewConfigError(message string, cause error) error {
	return &ConfigError{
		base: Error{
			code:    CodeConfig,
			message: message,
			err:     cause,
		},
	}
}

// UnauthorizedError represents authentication or permission errors.
type UnauthorizedError struct {
	base Error
}

func (e *UnauthorizedError) Error() string {
	return e.base.Error()
}

func (e *UnauthorizedError) Code() string {
	return e.base.Code()
}

func (e *UnauthorizedError) Unwrap() error {
	return e.base.Unwrap()
}

// NewUnauthorizedError creates a new error for authorization failures.
func NewUnauthorizedError(message string) error {
	return &UnauthorizedError{
		base: Error{
			code:    CodeUnauthorized,
			message: message,
		},
	}
}
