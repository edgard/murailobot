package main

import (
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

// Unwrap returns the original error.
func (fle *FileLineError) Unwrap() error {
	return fle.Err
}

// WrapError wraps an error with the file name and line number where it occurred.
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	} else {
		file = filepath.Base(file)
	}

	return &FileLineError{
		File: file,
		Line: line,
		Err:  err,
	}
}
