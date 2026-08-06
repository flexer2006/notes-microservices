package fault

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/logger"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrContextCanceled = errors.New("context was canceled during retry")
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	lastChange       time.Time
	Timeout          time.Duration
	IsFailure        func(error) bool
	name             string
	mu               sync.Mutex
	ErrorThreshold   int
	SuccessThreshold int
	failures         int
	successes        int
	state            CircuitState
	halfOpenInFlight bool
}

type Retry struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	ShouldRetry    func(error) bool
	name           string
	BackoffFactor  float64
	MaxAttempts    int
}

const (
	circuitErrorThreshold   = 5
	circuitSuccessThreshold = 2
	circuitTimeout          = 10 * time.Second
	retryMaxAttempts        = 3
	retryInitialBackoff     = 100 * time.Millisecond
	retryMaxBackoff         = 1 * time.Second
	retryBackoffFactor      = 2.0
)

type ServiceResilience struct {
	circuitBreaker *CircuitBreaker
	retry          *Retry
	serviceName    string
}

func NewServiceResilience(serviceName string, isInfraError func(error) bool) *ServiceResilience {
	shouldRetry := func(err error) bool {
		if errors.Is(err, context.Canceled) {
			return false
		}

		if isInfraError != nil {
			return isInfraError(err)
		}

		return !errors.Is(err, context.DeadlineExceeded)
	}

	return new(ServiceResilience{
		serviceName: serviceName,
		circuitBreaker: new(CircuitBreaker{
			lastChange:       time.Now(),
			Timeout:          circuitTimeout,
			IsFailure:        isInfraError,
			name:             serviceName,
			mu:               sync.Mutex{},
			ErrorThreshold:   circuitErrorThreshold,
			SuccessThreshold: circuitSuccessThreshold,
			failures:         0,
			successes:        0,
			state:            StateClosed,
			halfOpenInFlight: false,
		}),
		retry: new(Retry{
			InitialBackoff: retryInitialBackoff,
			MaxBackoff:     retryMaxBackoff,
			ShouldRetry:    shouldRetry,
			name:           serviceName,
			BackoffFactor:  retryBackoffFactor,
			MaxAttempts:    retryMaxAttempts,
		}),
	})
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	allowErr := cb.allow(ctx)
	if allowErr != nil {
		return allowErr
	}

	err := fn()
	cb.record(ctx, err)

	return err
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.state
}

func (r *Retry) Execute(ctx context.Context, operation func() error) error {
	_, err := retryExecute(ctx, r, func() (struct{}, error) {
		return struct{}{}, operation()
	})

	return err
}

func (r *ServiceResilience) ExecuteWithResilience(
	ctx context.Context,
	operationName string,
	operation func() error,
) error {
	r.log(ctx, operationName).Debug(ctx, "executing operation with resilience")

	return r.circuitBreaker.Execute(ctx, func() error {
		return r.retry.Execute(ctx, operation)
	})
}

func (r *ServiceResilience) ExecuteCircuitOnly(
	ctx context.Context,
	operationName string,
	operation func() error,
) error {
	r.log(ctx, operationName).Debug(ctx, "executing operation with circuit breaker only")

	return r.circuitBreaker.Execute(ctx, operation)
}

//nolint:ireturn // T is a type parameter; call sites pass concrete types, not interfaces.
func ExecuteWithResilienceResult[T any](
	ctx context.Context,
	resilience *ServiceResilience,
	operationName string,
	operation func() (T, error),
) (T, error) {
	resilience.log(ctx, operationName).Debug(ctx, "executing operation with resilience and result")

	var result T

	err := resilience.circuitBreaker.Execute(ctx, func() error {
		var opErr error

		result, opErr = retryExecute(ctx, resilience.retry, operation)
		if opErr != nil {
			resilience.log(ctx, operationName).Warn(ctx, "operation failed", zap.Error(opErr))
		}

		return opErr
	})

	return result, err
}

//nolint:ireturn // T is a type parameter; call sites pass concrete types, not interfaces.
func ExecuteCircuitOnlyResult[T any](
	ctx context.Context,
	resilience *ServiceResilience,
	operationName string,
	operation func() (T, error),
) (T, error) {
	resilience.log(ctx, operationName).Debug(ctx, "executing operation with circuit breaker only")

	var result T

	err := resilience.circuitBreaker.Execute(ctx, func() error {
		var opErr error

		result, opErr = operation()
		if opErr != nil {
			resilience.log(ctx, operationName).Warn(ctx, "operation failed", zap.Error(opErr))
		}

		return opErr
	})

	return result, err
}

func (cb *CircuitBreaker) allow(ctx context.Context) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	log := cb.log(ctx)
	switch cb.state {
	case StateOpen:
		switch {
		case time.Since(cb.lastChange) <= cb.Timeout:
			log.Info(ctx, "circuit breaker rejected request")

			return ErrCircuitOpen
		default:
			cb.state, cb.lastChange = StateHalfOpen, time.Now()
			cb.halfOpenInFlight = true

			log.Info(ctx, "circuit breaker state changed", zap.Int("new_state", int(StateHalfOpen)))
			log.Info(ctx, "circuit breaker allowing retry")
		}
	case StateHalfOpen:
		if cb.halfOpenInFlight {
			log.Info(ctx, "circuit breaker rejected concurrent half-open probe")

			return ErrCircuitOpen
		}

		cb.halfOpenInFlight = true

		log.Info(ctx, "circuit breaker allowing retry")
	case StateClosed:
	}

	return nil
}

func (cb *CircuitBreaker) record(ctx context.Context, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.halfOpenInFlight = false

	log := cb.log(ctx)
	if err != nil && cb.IsFailure != nil && !cb.IsFailure(err) {
		return
	}

	if err != nil {
		cb.failures++
		if cb.state == StateClosed && cb.failures >= cb.ErrorThreshold {
			cb.trip(ctx, log)

			return
		}

		if cb.state == StateHalfOpen {
			cb.trip(ctx, log)

			return
		}

		return
	}

	if cb.state == StateClosed {
		cb.failures = 0

		return
	}

	cb.successes++
	if cb.state == StateHalfOpen && cb.successes >= cb.SuccessThreshold {
		log.Info(ctx, "circuit breaker reset")

		cb.state, cb.lastChange, cb.failures, cb.successes = StateClosed, time.Now(), 0, 0
		log.Info(ctx, "circuit breaker state changed", zap.Int("new_state", int(StateClosed)))
	}
}

func (cb *CircuitBreaker) trip(ctx context.Context, log *logger.Logger) {
	log.Warn(ctx, "circuit breaker tripped", zap.Int("failures", cb.failures))
	cb.state, cb.lastChange, cb.successes = StateOpen, time.Now(), 0
	log.Info(ctx, "circuit breaker state changed", zap.Int("new_state", int(StateOpen)))
}

//nolint:ireturn
func retryExecute[T any](ctx context.Context, rt *Retry, operation func() (T, error)) (T, error) {
	log := logger.Log(ctx).With(zap.String("retry", rt.name))
	log.Debug(ctx, "retry operation")

	backoff := rt.InitialBackoff
	maxAttempts := max(rt.MaxAttempts, 1)

	var zero T

	attempt := 1

	for {
		res, err := operation()
		if err == nil || !rt.ShouldRetry(err) {
			if attempt > 1 && err == nil {
				log.Info(ctx, "retry succeeded", zap.Int("attempts", attempt))
			}

			return res, err
		}

		if attempt >= maxAttempts {
			log.Warn(
				ctx,
				"retry max attempts reached",
				zap.Int("attempts", attempt),
				zap.Error(err),
			)

			return res, err
		}

		log.Info(
			ctx,
			"retry attempt",
			zap.Int("attempt", attempt),
			zap.Duration("backoff", backoff),
			zap.Error(err),
		)

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()

			return zero, fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())
		}

		timer.Stop()

		backoff = min(time.Duration(float64(backoff)*rt.BackoffFactor), rt.MaxBackoff)
		attempt++
	}
}

func (r *ServiceResilience) log(ctx context.Context, operationName string) *logger.Logger {
	return logger.Log(ctx).
		With(zap.String("service", r.serviceName), zap.String("operation", operationName))
}

func (cb *CircuitBreaker) log(ctx context.Context) *logger.Logger {
	return logger.Log(ctx).
		With(zap.String("circuit_breaker", cb.name), zap.Int("circuit_state", int(cb.state)))
}
