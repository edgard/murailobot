// Package utils provides common utility functions and patterns
package utils

import (
	"context"
	"math/rand"
	"time"

	"github.com/sony/gobreaker"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

// String returns the string representation of CircuitState
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

// CircuitBreaker implements the circuit breaker pattern using gobreaker
type CircuitBreaker struct {
	name    string
	timeout time.Duration
	cb      *gobreaker.CircuitBreaker
}

// CircuitBreakerConfig holds configuration for circuit breakers
type CircuitBreakerConfig struct {
	Name          string
	MaxFailures   int
	Timeout       time.Duration
	HalfOpenLimit int
	ResetInterval time.Duration
	OnStateChange func(name string, from, to CircuitState)
}

// mapState converts gobreaker state to our CircuitState
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

// NewCircuitBreaker creates a new circuit breaker
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
			WriteInfoLog("resilience", "Circuit breaker state changed",
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

// Execute runs an operation through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func(context.Context) error) error {
	// Create timeout context if none provided
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.timeout)
		defer cancel()
	}

	WriteDebugLog("resilience", "executing operation through circuit breaker",
		KeyName, cb.name,
		KeyAction, "circuit_breaker_execute",
		KeyType, "circuit_breaker")

	_, err := cb.cb.Execute(func() (interface{}, error) {
		err := operation(ctx)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, NewError("resilience", ErrTimeout, "operation timed out", CategoryOperation, err)
			}
			return nil, err
		}
		WriteDebugLog("resilience", "operation completed successfully",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_success",
			KeyType, "circuit_breaker")
		return nil, nil
	})

	if err != nil {
		WriteDebugLog("resilience", "operation failed",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_failure",
			KeyType, "circuit_breaker",
			KeyError, err.Error())
	}
	return err
}

// RetryConfig holds configuration for retry operations
type RetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	RandomFactor    float64
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		RandomFactor:    0.1,
	}
}

// WithRetry executes an operation with exponential backoff retry
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

		// Don't retry if context is done
		if ctx.Err() != nil {
			return NewError("resilience", ErrTimeout, "retry abandoned: context done", CategoryOperation, ctx.Err())
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

			WriteDebugLog("resilience", "Operation failed, retrying",
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
				return NewError("resilience", ErrTimeout, "retry abandoned: context done", CategoryOperation, ctx.Err())
			case <-timer.C:
				timer.Stop()
			}
		}
	}

	return Errorf("resilience", ErrOperation, CategoryOperation,
		"retry attempts exhausted after %d tries: %v", cfg.MaxAttempts, lastErr)
}
