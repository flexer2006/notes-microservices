package fault_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

var (
	errInfra = errors.New("infra")
	errBiz   = errors.New("biz")
	errTmp   = errors.New("tmp")
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

func isInfra(err error) bool {
	return errors.Is(err, errInfra)
}

func TestExecuteCircuitOnlySuccess(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	err := resilience.ExecuteCircuitOnly(context.Background(), "ok", func() error { return nil })
	testkit.MyErrIs(t, err, nil)
}

func TestExecuteCircuitOnlyResult(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	got, err := fault.ExecuteCircuitOnlyResult(
		context.Background(),
		resilience,
		"ok",
		func() (int, error) { return 7, nil },
	)
	testkit.MyErrIs(t, err, nil)

	if got != 7 {
		t.Fatalf("got %d", got)
	}
}

func TestExecuteCircuitOnlyResultError(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	_, err := fault.ExecuteCircuitOnlyResult(
		context.Background(),
		resilience,
		"fail",
		func() (int, error) { return 0, errInfra },
	)
	testkit.MyErrIs(t, err, errInfra)
}

func TestCircuitOpensAfterThreshold(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	ctx := context.Background()

	for range 5 {
		err := resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
		testkit.MyErrIs(t, err, errInfra)
	}

	err := resilience.ExecuteCircuitOnly(ctx, "blocked", func() error { return nil })
	testkit.MyErrIs(t, err, fault.ErrCircuitOpen)
}

func TestCircuitIgnoresNonInfraErrors(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	ctx := context.Background()

	for range 10 {
		err := resilience.ExecuteCircuitOnly(ctx, "biz", func() error { return errBiz })
		testkit.MyErrIs(t, err, errBiz)
	}

	testkit.MyErrIs(t, resilience.ExecuteCircuitOnly(ctx, "ok", func() error { return nil }), nil)
}

func TestCircuitHalfOpenAndReset(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		ctx := context.Background()

		for range 5 {
			_ = resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
		}

		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "open", func() error { return nil }),
			fault.ErrCircuitOpen,
		)

		time.Sleep(10*time.Second + time.Nanosecond)
		synctest.Wait()

		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "probe1", func() error { return nil }),
			nil,
		)
		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "probe2", func() error { return nil }),
			nil,
		)
		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "closed", func() error { return nil }),
			nil,
		)
	})
}

func TestCircuitHalfOpenRejectsConcurrentProbe(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		ctx := context.Background()

		for range 5 {
			_ = resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
		}

		time.Sleep(10*time.Second + time.Nanosecond)
		synctest.Wait()

		release := make(chan struct{})
		started := make(chan struct{})

		go func() {
			close(started)

			_ = resilience.ExecuteCircuitOnly(ctx, "hold", func() error {
				<-release

				return nil
			})
		}()

		<-started
		synctest.Wait()

		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "busy", func() error { return nil }),
			fault.ErrCircuitOpen,
		)
		close(release)
		synctest.Wait()
	})
}

func TestCircuitHalfOpenFailureRetracts(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		ctx := context.Background()

		for range 5 {
			_ = resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
		}

		time.Sleep(10*time.Second + time.Nanosecond)
		synctest.Wait()

		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "probe", func() error { return errInfra }),
			errInfra,
		)
		testkit.MyErrIs(
			t,
			resilience.ExecuteCircuitOnly(ctx, "open", func() error { return nil }),
			fault.ErrCircuitOpen,
		)
	})
}

func TestCircuitBreakerGetState(t *testing.T) {
	t.Parallel()

	breaker := fault.CircuitBreaker{
		Timeout:          time.Second,
		IsFailure:        isInfra,
		ErrorThreshold:   2,
		SuccessThreshold: 1,
	}
	ctx := context.Background()

	if breaker.GetState() != fault.StateClosed {
		t.Fatalf("state = %v", breaker.GetState())
	}

	_ = breaker.Execute(ctx, func() error { return errInfra })
	_ = breaker.Execute(ctx, func() error { return errInfra })

	if breaker.GetState() != fault.StateOpen {
		t.Fatalf("state = %v", breaker.GetState())
	}
}

func TestRetryEventuallySucceeds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)

		var attempts atomic.Int32

		err := resilience.ExecuteWithResilience(context.Background(), "retry", func() error {
			if attempts.Add(1) < 3 {
				return errInfra
			}

			return nil
		})
		testkit.MyErrIs(t, err, nil)

		if attempts.Load() != 3 {
			t.Fatalf("attempts = %d", attempts.Load())
		}
	})
}

func TestRetryResult(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)

		var attempts atomic.Int32

		got, err := fault.ExecuteWithResilienceResult(
			context.Background(),
			resilience,
			"retry",
			func() (string, error) {
				if attempts.Add(1) < 2 {
					return "", errInfra
				}

				return "ok", nil
			},
		)
		testkit.MyErrIs(t, err, nil)

		if got != "ok" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestRetryResultExhausted(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		_, err := fault.ExecuteWithResilienceResult(
			context.Background(),
			resilience,
			"fail",
			func() (string, error) { return "", errInfra },
		)
		testkit.MyErrIs(t, err, errInfra)
	})
}

func TestRetryExhausted(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		err := resilience.ExecuteWithResilience(
			context.Background(),
			"fail",
			func() error { return errInfra },
		)
		testkit.MyErrIs(t, err, errInfra)
	})
}

func TestRetryCanceledDuringBackoff(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", isInfra)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err := resilience.ExecuteWithResilience(ctx, "cancel", func() error { return errInfra })
		testkit.MyErrIs(t, err, fault.ErrContextCanceled)
	})
}

func TestRetrySkipsCanceledError(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)

	var attempts atomic.Int32

	err := resilience.ExecuteWithResilience(context.Background(), "canceled", func() error {
		attempts.Add(1)

		return context.Canceled
	})
	testkit.MyErrIs(t, err, context.Canceled)

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}

func TestRetryNilClassifier(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		resilience := fault.NewServiceResilience("svc", nil)

		var attempts atomic.Int32

		err := resilience.ExecuteWithResilience(context.Background(), "nil-class", func() error {
			if attempts.Add(1) < 2 {
				return errTmp
			}

			return nil
		})
		testkit.MyErrIs(t, err, nil)
	})
}

func TestRetryDirectMaxAttemptsClamp(t *testing.T) {
	t.Parallel()

	retry := new(fault.Retry{
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		ShouldRetry:    func(error) bool { return true },
		BackoffFactor:  2,
		MaxAttempts:    0,
	})

	var attempts atomic.Int32

	err := retry.Execute(context.Background(), func() error {
		attempts.Add(1)

		return errInfra
	})
	testkit.MyErrIs(t, err, errInfra)

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}

func TestClosedSuccessResetsFailures(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", isInfra)
	ctx := context.Background()

	for range 4 {
		_ = resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
	}

	testkit.MyErrIs(t, resilience.ExecuteCircuitOnly(ctx, "ok", func() error { return nil }), nil)

	for range 4 {
		_ = resilience.ExecuteCircuitOnly(ctx, "fail", func() error { return errInfra })
	}

	testkit.MyErrIs(
		t,
		resilience.ExecuteCircuitOnly(ctx, "still-closed", func() error { return nil }),
		nil,
	)
}

func TestNilClassifierDoesNotRetryDeadline(t *testing.T) {
	t.Parallel()

	resilience := fault.NewServiceResilience("svc", nil)

	var attempts atomic.Int32

	err := resilience.ExecuteWithResilience(context.Background(), "deadline", func() error {
		attempts.Add(1)

		return context.DeadlineExceeded
	})
	testkit.MyErrIs(t, err, context.DeadlineExceeded)

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}
