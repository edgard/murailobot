package ai

import (
	"errors"
	"time"
)

var (
	// ErrAI represents AI service related errors
	ErrAI = errors.New("ai service error")

	// List of errors that should not be retried
	invalidRequestErrors = []string{
		"invalid_request_error",
		"context_length_exceeded",
		"rate_limit_exceeded",
	}
)

// Config holds configuration for the AI service
type Config struct {
	Token       string
	BaseURL     string
	Model       string
	Temperature float32
	TopP        float32
	Instruction string
	Timeout     time.Duration
}
