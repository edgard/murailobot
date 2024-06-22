package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

// FileLineError wraps an error with the file name and line number where it occurred.
type FileLineError struct {
	File string // Name of the file where the error occurred
	Line int    // Line number in the file where the error occurred
	Err  error  // The original error
}

// Error implements the error interface for FileLineError.
func (fle *FileLineError) Error() string {
	return fmt.Sprintf("[%s:%d] %v", fle.File, fle.Line, fle.Err)
}

// WrapError wraps an error with the file name and line number where it occurred, and a custom message.
func WrapError(message string, err ...error) error {
	var originalErr error
	if len(err) > 0 {
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
