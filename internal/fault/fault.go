package fault

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	lastChange                                            time.Time
	mu                                                    sync.Mutex
	name                                                  string
	ErrorThreshold, SuccessThreshold, failures, successes int
	Timeout                                               time.Duration
	state                                                 CircuitState
}

type Retry struct {
	name           string
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	ShouldRetry    func(error) bool
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
	serviceName    string
	circuitBreaker *CircuitBreaker
	retry          *Retry
}

func NewServiceResilience(serviceName string) *ServiceResilience {
	return new(ServiceResilience{
		serviceName: serviceName,
		circuitBreaker: new(CircuitBreaker{
			name:             serviceName,
			ErrorThreshold:   circuitErrorThreshold,
			Timeout:          circuitTimeout,
			SuccessThreshold: circuitSuccessThreshold,
			state:            StateClosed,
			lastChange:       time.Now(),
		}),
		retry: new(Retry{
			name:           serviceName,
			MaxAttempts:    retryMaxAttempts,
			InitialBackoff: retryInitialBackoff,
			MaxBackoff:     retryMaxBackoff,
			BackoffFactor:  retryBackoffFactor,
			ShouldRetry: func(err error) bool {
				return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
			},
		}),
	})
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) (err error) {
	if err = cb.allow(ctx); err != nil {
		return err
	}
	err = fn()
	cb.record(ctx, err)
	return
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func (r *Retry) Execute(ctx context.Context, operation func() error) error {
	_, err := retryExecute(r, ctx, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func (r *ServiceResilience) ExecuteWithResilience(ctx context.Context, operationName string, operation func() error) error {
	r.log(ctx, operationName).Debug(ctx, "executing operation with resilience")
	return r.circuitBreaker.Execute(ctx, func() error {
		return r.retry.Execute(ctx, operation)
	})
}

func ExecuteWithResilienceResult[T any](r *ServiceResilience, ctx context.Context, operationName string, operation func() (T, error)) (T, error) {
	r.log(ctx, operationName).Debug(ctx, "executing operation with resilience and result")
	var result T
	err := r.circuitBreaker.Execute(ctx, func() error {
		var err error
		result, err = retryExecute(r.retry, ctx, operation)
		if err != nil {
			r.log(ctx, operationName).Warn(ctx, "operation failed", zap.Error(err))
		}
		return err
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
			return domain.ErrCircuitOpen
		default:
			cb.state, cb.lastChange = StateHalfOpen, time.Now()
			log.Info(ctx, "circuit breaker state changed", zap.Int("new_state", int(StateHalfOpen)))
			log.Info(ctx, "circuit breaker allowing retry")
		}
	case StateHalfOpen:
		log.Info(ctx, "circuit breaker allowing retry")
	}
	return nil
}

func (cb *CircuitBreaker) record(ctx context.Context, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log := cb.log(ctx)
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
	if cb.state == StateOpen {
		return
	}
	log.Warn(ctx, "circuit breaker tripped", zap.Int("failures", cb.failures))
	cb.state, cb.lastChange, cb.successes = StateOpen, time.Now(), 0
	log.Info(ctx, "circuit breaker state changed", zap.Int("new_state", int(StateOpen)))
}

func retryExecute[T any](r *Retry, ctx context.Context, operation func() (T, error)) (T, error) {
	log := logger.Log(ctx).With(zap.String("retry", r.name))
	log.Debug(ctx, "retry operation")
	backoff := r.InitialBackoff
	var zero T
	for attempt := 1; attempt <= r.MaxAttempts; attempt++ {
		res, err := operation()
		if err == nil || !r.ShouldRetry(err) {
			if attempt > 1 && err == nil {
				log.Info(ctx, "retry succeeded", zap.Int("attempts", attempt))
			}
			return res, err
		}
		if attempt >= r.MaxAttempts {
			log.Warn(ctx, "retry max attempts reached", zap.Int("attempts", attempt), zap.Error(err))
			return res, err
		}
		log.Info(ctx, "retry attempt", zap.Int("attempt", attempt), zap.Duration("backoff", backoff), zap.Error(err))
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return zero, fmt.Errorf("%w: %w", domain.ErrContextCanceled, ctx.Err())
		}
		timer.Stop()
		backoff = min(time.Duration(float64(backoff)*r.BackoffFactor), r.MaxBackoff)
	}
	return zero, nil
}

func (r *ServiceResilience) log(ctx context.Context, operationName string) *logger.Logger {
	return logger.Log(ctx).With(zap.String("service", r.serviceName), zap.String("operation", operationName))
}

func (cb *CircuitBreaker) log(ctx context.Context) *logger.Logger {
	return logger.Log(ctx).With(zap.String("circuit_breaker", cb.name), zap.Int("circuit_state", int(cb.state)))
}
