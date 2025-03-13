package errors

import (
	"errors"
	"fmt"
)

// Standard error codes for the application.
const (
	CodeUnknown      = "UNKNOWN"
	CodeDatabase     = "DATABASE"
	CodeValidation   = "VALIDATION"
	CodeAPI          = "API"
	CodeConfig       = "CONFIG"
	CodeUnauthorized = "UNAUTHORIZED"
)

// ApplicationError is the interface that all our custom errors implement.
type ApplicationError interface {
	error
	Code() string
	Unwrap() error
}

// Error represents a basic application error.
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

// or CodeUnknown if it doesn't.
func Code(err error) string {
	var appErr ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Code()
	}

	return CodeUnknown
}

// Specific error types and constructors

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

func NewDatabaseError(message string, cause error) error {
	return &DatabaseError{
		base: Error{
			code:    CodeDatabase,
			message: message,
			err:     cause,
		},
	}
}

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

func NewValidationError(message string, cause error) error {
	return &ValidationError{
		base: Error{
			code:    CodeValidation,
			message: message,
			err:     cause,
		},
	}
}

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

func NewAPIError(message string, cause error) error {
	return &APIError{
		base: Error{
			code:    CodeAPI,
			message: message,
			err:     cause,
		},
	}
}

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

func NewConfigError(message string, cause error) error {
	return &ConfigError{
		base: Error{
			code:    CodeConfig,
			message: message,
			err:     cause,
		},
	}
}

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

func NewUnauthorizedError(message string) error {
	return &UnauthorizedError{
		base: Error{
			code:    CodeUnauthorized,
			message: message,
		},
	}
}
