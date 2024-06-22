package main

import (
	"fmt"
	"runtime"
)

type FuncError struct {
	Func string
	Err  error
}

func (fe *FuncError) Error() string {
	return fmt.Sprintf("%s: %v", fe.Func, fe.Err)
}

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
