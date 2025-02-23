// Package utils provides common utility functions and patterns.
// This file implements resilience patterns including circuit breakers
// and retries with exponential backoff to handle transient failures
// and prevent cascading failures in distributed systems.
package utils

import (
	"context"
	"math/rand"
	"time"

	"github.com/sony/gobreaker"
)

const resilienceComponent = "resilience"

// CircuitState represents the state of a circuit breaker.
// The circuit breaker can be in one of three states:
// - Closed: Normal operation, requests are allowed
// - Half-Open: Testing if service has recovered
// - Open: Requests are blocked to prevent cascading failures
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation
	StateHalfOpen                     // Testing recovery
	StateOpen                         // Failing fast
)

// String returns the string representation of CircuitState.
// This is used for logging and debugging purposes.
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF-OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker implements the circuit breaker pattern to prevent
// cascading failures in distributed systems. It tracks operation
// failures and temporarily blocks operations when failure thresholds
// are exceeded.
type CircuitBreaker struct {
	name    string                    // Identifier for logging
	timeout time.Duration             // Default operation timeout
	cb      *gobreaker.CircuitBreaker // Underlying circuit breaker
}

// CircuitBreakerConfig holds configuration for circuit breakers.
// It allows fine-tuning of circuit breaker behavior to match
// the characteristics of the protected resource.
type CircuitBreakerConfig struct {
	Name          string                                   // Identifier for the circuit breaker
	MaxFailures   int                                      // Number of failures before opening
	Timeout       time.Duration                            // Default operation timeout
	HalfOpenLimit int                                      // Max requests in half-open state
	ResetInterval time.Duration                            // Time before attempting recovery
	OnStateChange func(name string, from, to CircuitState) // State change callback
}

// mapState converts gobreaker state to our CircuitState type.
// This allows us to abstract the underlying circuit breaker
// implementation from the rest of the application.
func mapState(state gobreaker.State) CircuitState {
	switch state {
	case gobreaker.StateClosed:
		return StateClosed
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	case gobreaker.StateOpen:
		return StateOpen
	default:
		return StateClosed
	}
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
// It provides sensible defaults if configuration values are not specified:
// - MaxFailures: 5 consecutive failures before opening
// - Timeout: 30 seconds default operation timeout
// - HalfOpenLimit: 1 test request when half-open
// - ResetInterval: 60 seconds before attempting recovery
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.HalfOpenLimit <= 0 {
		cfg.HalfOpenLimit = 1
	}
	if cfg.ResetInterval <= 0 {
		cfg.ResetInterval = 60 * time.Second
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: uint32(cfg.HalfOpenLimit),
		Interval:    cfg.ResetInterval,
		Timeout:     cfg.ResetInterval,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureCount := counts.ConsecutiveFailures
			return failureCount >= uint32(cfg.MaxFailures)
		},
	}

	if cfg.OnStateChange != nil {
		settings.OnStateChange = func(name string, from, to gobreaker.State) {
			fromState := mapState(from)
			toState := mapState(to)
			cfg.OnStateChange(name, fromState, toState)
			WriteInfoLog(resilienceComponent, "Circuit breaker state changed",
				KeyName, name,
				KeyFrom, fromState.String(),
				KeyTo, toState.String(),
				KeyType, "circuit_breaker",
				KeyAction, "state_change")
		}
	}

	return &CircuitBreaker{
		name:    cfg.Name,
		timeout: cfg.Timeout,
		cb:      gobreaker.NewCircuitBreaker(settings),
	}
}

// Execute runs an operation through the circuit breaker with timeout handling.
// If the context doesn't have a deadline, it applies the circuit breaker's
// default timeout. The operation is only executed if the circuit breaker
// is in a state that allows it (Closed or testing in Half-Open).
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func(context.Context) error) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.timeout)
		defer cancel()
	}

	WriteDebugLog(resilienceComponent, "executing operation through circuit breaker",
		KeyName, cb.name,
		KeyAction, "circuit_breaker_execute",
		KeyType, "circuit_breaker")

	_, err := cb.cb.Execute(func() (interface{}, error) {
		err := operation(ctx)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, NewError(resilienceComponent, ErrTimeout, "operation timed out", CategoryOperation, err)
			}
			return nil, err
		}
		WriteDebugLog(resilienceComponent, "operation completed successfully",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_success",
			KeyType, "circuit_breaker")
		return nil, nil
	})

	if err != nil {
		WriteDebugLog(resilienceComponent, "operation failed",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_failure",
			KeyType, "circuit_breaker",
			KeyError, err.Error())
	}
	return err
}

// RetryConfig holds configuration for retry operations.
// It defines the backoff strategy for retrying failed operations.
type RetryConfig struct {
	MaxAttempts     int           // Maximum number of attempts
	InitialInterval time.Duration // Starting delay between retries
	MaxInterval     time.Duration // Maximum delay between retries
	Multiplier      float64       // Factor to increase delay after each retry
	RandomFactor    float64       // Randomization factor (0-1) for jitter
}

// DefaultRetryConfig returns a default retry configuration:
// - 3 maximum attempts
// - 100ms initial interval
// - 30s maximum interval
// - 2.0 multiplier (exponential backoff)
// - 0.1 random factor (10% jitter)
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		RandomFactor:    0.1,
	}
}

// WithRetry executes an operation with exponential backoff retry.
// It implements the following retry strategy:
// - Exponential backoff between attempts
// - Jitter to prevent thundering herd
// - Maximum retry interval cap
// - Context cancellation handling
// - Special error handling (e.g., circuit breaker open)
func WithRetry(ctx context.Context, operation func(context.Context) error, cfg RetryConfig) error {
	var lastErr error
	interval := cfg.InitialInterval
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := operation(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return NewError(resilienceComponent, ErrTimeout, "retry abandoned: context done", CategoryOperation, ctx.Err())
		}

		// Don't retry on certain errors
		if err == gobreaker.ErrOpenState {
			return err
		}

		if attempt < cfg.MaxAttempts {
			// Calculate next interval with jitter
			jitter := 1.0 + (cfg.RandomFactor * (2*rnd.Float64() - 1))
			interval = time.Duration(float64(interval) * cfg.Multiplier * jitter)
			if interval > cfg.MaxInterval {
				interval = cfg.MaxInterval
			}

			WriteDebugLog(resilienceComponent, "Operation failed, retrying",
				KeyType, "retry",
				KeyAction, "retry_operation",
				KeyCount, attempt,
				KeyLimit, cfg.MaxAttempts,
				KeySize, interval,
				KeyError, err.Error(),
				"retry_config", map[string]interface{}{
					"multiplier":    cfg.Multiplier,
					"random_factor": cfg.RandomFactor,
					"max_interval":  cfg.MaxInterval,
					"next_interval": interval,
				})

			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return NewError(resilienceComponent, ErrTimeout, "retry abandoned: context done", CategoryOperation, ctx.Err())
			case <-timer.C:
				timer.Stop()
			}
		}
	}

	return Errorf(resilienceComponent, ErrOperation, CategoryOperation,
		"retry attempts exhausted after %d tries: %v", cfg.MaxAttempts, lastErr)
}
