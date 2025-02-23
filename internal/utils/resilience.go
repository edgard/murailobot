// Package utils provides circuit breaker and retry patterns.
package utils

import (
	"context"
	"math/rand"
	"time"

	"github.com/sony/gobreaker"
)

const resilienceComponent = "resilience"

// CircuitState represents circuit breaker states:
// - Closed: Normal operation
// - Half-Open: Testing recovery
// - Open: Failing fast
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

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

type CircuitBreaker struct {
	name    string
	timeout time.Duration
	cb      *gobreaker.CircuitBreaker
}

type CircuitBreakerConfig struct {
	Name          string
	MaxFailures   int
	Timeout       time.Duration
	HalfOpenLimit int
	ResetInterval time.Duration
	OnStateChange func(name string, from, to CircuitState)
}

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

// NewCircuitBreaker creates circuit breaker with defaults:
// - 5 failures before opening
// - 30s timeout
// - 1 test request when half-open
// - 60s before recovery attempt
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
			InfoLog(resilienceComponent, "Circuit breaker state changed",
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

// Execute runs operation with timeout and circuit breaking
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func(context.Context) error) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.timeout)
		defer cancel()
	}

	DebugLog(resilienceComponent, "executing operation through circuit breaker",
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
		DebugLog(resilienceComponent, "operation completed successfully",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_success",
			KeyType, "circuit_breaker")
		return nil, nil
	})

	if err != nil {
		DebugLog(resilienceComponent, "operation failed",
			KeyName, cb.name,
			KeyAction, "circuit_breaker_failure",
			KeyType, "circuit_breaker",
			KeyError, err.Error())
	}
	return err
}

type RetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	RandomFactor    float64
}

// DefaultRetryConfig returns:
// - 3 attempts
// - 100ms initial delay
// - 30s max delay
// - 2.0x multiplier
// - 10% jitter
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		RandomFactor:    0.1,
	}
}

// WithRetry implements exponential backoff with:
// - Jitter to prevent thundering herd
// - Maximum interval cap
// - Context cancellation handling
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

		if err == gobreaker.ErrOpenState {
			return err
		}

		if attempt < cfg.MaxAttempts {
			jitter := 1.0 + (cfg.RandomFactor * (2*rnd.Float64() - 1))
			interval = time.Duration(float64(interval) * cfg.Multiplier * jitter)
			if interval > cfg.MaxInterval {
				interval = cfg.MaxInterval
			}

			DebugLog(resilienceComponent, "Operation failed, retrying",
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
