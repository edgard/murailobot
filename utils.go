package main

import (
	"fmt"
	"runtime"
)

// FuncError wraps an error with the function name where it occurred.
type FuncError struct {
	Func string // Name of the function where the error occurred
	Err  error  // The original error
}

// Error implements the error interface for FuncError.
func (fe *FuncError) Error() string {
	return fmt.Sprintf("%s: %v", fe.Func, fe.Err)
}

// WrapError wraps an error with the name of the function where it occurred.
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	pc, _, _, ok := runtime.Caller(1)
	funcName := "unknown"
	if ok {
		funcName = runtime.FuncForPC(pc).Name()
	}

	return &FuncError{
		Func: funcName,
		Err:  err,
	}
}
