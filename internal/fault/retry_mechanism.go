package fault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
)

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	ShouldRetry    func(error) bool
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
		ShouldRetry:    defaultShouldRetry,
	}
}

func defaultShouldRetry(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

type Retry struct {
	config RetryConfig
	name   string
}

func NewRetry(name string, config RetryConfig) *Retry {
	return new(Retry{
		name:   name,
		config: config,
	})
}

func (r *Retry) Execute(ctx context.Context, operation func() error) error {
	log := logger.Log(ctx).With(zap.String("retry", r.name))
	log.Debug(ctx, domain.LogRetryOperation)
	var err error
	backoff := r.config.InitialBackoff
	attempts := 0
	for attempts < r.config.MaxAttempts {
		attempts++
		err = operation()
		if err == nil || !r.config.ShouldRetry(err) {
			if attempts > 1 && err == nil {
				log.Info(ctx, domain.LogRetrySuccess, zap.Int("attempts", attempts))
			}
			return err
		}
		if attempts >= r.config.MaxAttempts {
			log.Warn(ctx, domain.LogRetryMaxAttempts,
				zap.Int("attempts", attempts),
				zap.Error(err))
			return err
		}
		log.Info(ctx, domain.LogRetryAttempt,
			zap.Int("attempt", attempts),
			zap.Duration("backoff", backoff),
			zap.Error(err))
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", domain.ErrContextCanceled, ctx.Err())
		}
		backoff = min(time.Duration(float64(backoff)*r.config.BackoffFactor), r.config.MaxBackoff)
	}
	return err
}
