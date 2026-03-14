package fault

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
)

type ServiceResilience struct {
	serviceName    string
	circuitBreaker *CircuitBreaker
	retry          *Retry
}

func NewServiceResilience(serviceName string) *ServiceResilience {
	return new(ServiceResilience{
		serviceName:    serviceName,
		circuitBreaker: NewCircuitBreaker(serviceName, DefaultCircuitBreakerConfig()),
		retry:          NewRetry(serviceName, DefaultRetryConfig()),
	})
}

func (r *ServiceResilience) ExecuteWithResilience(
	ctx context.Context,
	operationName string,
	operation func() error,
) error {
	log := logger.Log(ctx).With(
		zap.String("service", r.serviceName),
		zap.String("operation", operationName),
	)
	log.Debug(ctx, "Executing operation with resilience")
	return r.circuitBreaker.Execute(ctx, func() error {
		return r.retry.Execute(ctx, operation)
	})
}

func (r *ServiceResilience) ExecuteWithResultTokenResponse(
	ctx context.Context,
	operationName string,
	operation func() (any, error),
) (any, error) {
	log := logger.Log(ctx).With(
		zap.String("service", r.serviceName),
		zap.String("operation", operationName),
	)
	log.Debug(ctx, "Executing operation with resilience and result")
	var result any
	var resultErr error
	err := r.circuitBreaker.Execute(ctx, func() error {
		return r.retry.Execute(ctx, func() error {
			var err error
			result, err = operation()
			if err != nil {
				log.Warn(ctx, "Operation failed", zap.Error(err))
				return err
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}
