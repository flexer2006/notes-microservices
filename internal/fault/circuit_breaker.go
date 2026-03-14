package fault

import (
	"context"
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

type CircuitBreakerConfig struct {
	ErrorThreshold   int
	Timeout          time.Duration
	SuccessThreshold int
}

func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ErrorThreshold:   5,
		Timeout:          10 * time.Second,
		SuccessThreshold: 2,
	}
}

type CircuitBreaker struct {
	mu              sync.RWMutex
	config          CircuitBreakerConfig
	lastStateChange time.Time
	name            string
	state           CircuitState
	failures        int
	successes       int
}

func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	return new(CircuitBreaker{
		name:            name,
		state:           StateClosed,
		config:          config,
		failures:        0,
		successes:       0,
		lastStateChange: time.Now(),
	})
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.AllowRequest(ctx) {
		return domain.ErrCircuitOpen
	}
	err := fn()
	cb.RecordResult(ctx, err)
	return err
}

func (cb *CircuitBreaker) AllowRequest(ctx context.Context) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	log := logger.Log(ctx).With(
		zap.String("circuit_breaker", cb.name),
		zap.Int("circuit_state", int(cb.state)),
	)
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastStateChange) > cb.config.Timeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			defer cb.mu.Unlock()
			if cb.state == StateOpen && time.Since(cb.lastStateChange) > cb.config.Timeout {
				cb.state = StateHalfOpen
				cb.lastStateChange = time.Now()
				log.Info(ctx, domain.LogCircuitStateChange, zap.Int("new_state", int(StateHalfOpen)))
				log.Info(ctx, domain.LogCircuitAllowRetry)
				return true
			}
			return false
		}
		log.Info(ctx, domain.LogCircuitReject)
		return false
	case StateHalfOpen:
		log.Info(ctx, domain.LogCircuitAllowRetry)
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordResult(ctx context.Context, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log := logger.Log(ctx).With(
		zap.String("circuit_breaker", cb.name),
		zap.Int("circuit_state", int(cb.state)),
	)
	if err != nil {
		cb.onFailure(ctx, log)
		return
	}
	cb.onSuccess(ctx, log)
}

func (cb *CircuitBreaker) onFailure(ctx context.Context, log *logger.Logger) {
	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.config.ErrorThreshold {
			cb.tripBreaker(ctx, log)
		}
	case StateHalfOpen:
		cb.tripBreaker(ctx, log)
	}
}

func (cb *CircuitBreaker) onSuccess(ctx context.Context, log *logger.Logger) {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.resetBreaker(ctx, log)
		}
	}
}

func (cb *CircuitBreaker) tripBreaker(ctx context.Context, log *logger.Logger) {
	if cb.state != StateOpen {
		log.Warn(ctx, domain.LogCircuitTrip, zap.Int("failures", cb.failures))
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		cb.successes = 0
		log.Info(ctx, domain.LogCircuitStateChange, zap.Int("new_state", int(StateOpen)))
	}
}

func (cb *CircuitBreaker) resetBreaker(ctx context.Context, log *logger.Logger) {
	log.Info(ctx, domain.LogCircuitReset)
	cb.state = StateClosed
	cb.lastStateChange = time.Now()
	cb.failures = 0
	cb.successes = 0
	log.Info(ctx, domain.LogCircuitStateChange, zap.Int("new_state", int(StateClosed)))
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
