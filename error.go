package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

// FileLineError wraps an error with file and line number information.
type FileLineError struct {
	File string // Name of the file where the error occurred.
	Line int    // Line number where the error occurred.
	Err  error  // The underlying error.
}

// Error implements the error interface.
func (fle *FileLineError) Error() string {
	return fmt.Sprintf("[%s:%d] %v", fle.File, fle.Line, fle.Err)
}

// Unwrap returns the underlying error.
func (fle *FileLineError) Unwrap() error {
	return fle.Err
}

// WrapError wraps an error with a custom message and attaches file and line information.
// If no error is provided, it creates a new one using the message.
func WrapError(message string, err ...error) error {
	var originalErr error
	if len(err) > 0 && err[0] != nil {
		originalErr = err[0]
	}

	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	} else {
		file = filepath.Base(file)
	}

	if originalErr != nil {
		return &FileLineError{
			File: file,
			Line: line,
			Err:  fmt.Errorf("%s: %w", message, originalErr),
		}
	}

	return &FileLineError{
		File: file,
		Line: line,
		Err:  errors.New(message),
	}
}
