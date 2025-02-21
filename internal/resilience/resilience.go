// Package resilience provides standardized circuit breaker implementations with:
//   - Timeout handling with context support
//   - Retry mechanism with exponential backoff
//   - Context cancellation handling
//   - Structured logging
//   - Type-safe state management
//   - Sensible defaults
package resilience

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/sony/gobreaker"
)

var (
	// ErrCircuitOpen indicates the circuit breaker is open
	ErrCircuitOpen = gobreaker.ErrOpenState
	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")
	// ErrExhaustedRetries indicates retry attempts were exhausted
	ErrExhaustedRetries = errors.New("retry attempts exhausted")
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
			slog.Info("Circuit breaker state changed",
				"name", name,
				"from", fromState,
				"to", toState,
			)
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

	_, err := cb.cb.Execute(func() (interface{}, error) {
		err := operation(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
			}
			return nil, err
		}
		return nil, nil
	})

	if err != nil {
		return err
	}

	return nil
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
			return fmt.Errorf("retry abandoned: %w", ctx.Err())
		}

		// Don't retry on certain errors
		if errors.Is(err, ErrCircuitOpen) {
			return err
		}

		if attempt < cfg.MaxAttempts {
			// Calculate next interval with jitter
			jitter := 1.0 + (cfg.RandomFactor * (2*rnd.Float64() - 1))
			interval = time.Duration(float64(interval) * cfg.Multiplier * jitter)
			if interval > cfg.MaxInterval {
				interval = cfg.MaxInterval
			}

			slog.Debug("Operation failed, retrying",
				"attempt", attempt,
				"max_attempts", cfg.MaxAttempts,
				"next_interval", interval,
				"error", err,
			)

			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("retry abandoned: %w", ctx.Err())
			case <-timer.C:
				timer.Stop()
			}
		}
	}

	return fmt.Errorf("%w after %d attempts: %v",
		ErrExhaustedRetries,
		cfg.MaxAttempts,
		lastErr)
}
