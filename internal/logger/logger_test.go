package logger_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/flexer2006/notes-microservices/internal/logger"
)

var errSyncBoom = errors.New("sync boom")

type errSyncWriter struct{}

func (errSyncWriter) Write(p []byte) (int, error) { return len(p), nil }

func (errSyncWriter) Sync() error { return errSyncBoom }

func TestNewLogger(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, env, level string
	}{
		{name: "prod_info", env: logger.Production, level: "info"},
		{name: "dev_debug", env: logger.Development, level: "debug"},
		{name: "prod_bad_level", env: logger.Production, level: "nope"},
		{name: "dev_bad_level", env: logger.Development, level: "nope"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lg, err := logger.NewLogger(tc.env, tc.level)
			if err != nil || lg == nil {
				t.Fatalf("NewLogger: lg=%v err=%v", lg, err)
			}

			_ = lg.Sync()
		})
	}
}

func TestGlobalFallbackThenSet(t *testing.T) {
	t.Parallel()

	// Still safe under parallel: SetGlobalLogger is idempotent for tests using zap.Nop.
	first := logger.Global()
	if first == nil {
		t.Fatal("Global fallback nil")
	}

	nop := new(logger.Logger{Logger: zap.NewNop()})
	logger.SetGlobalLogger(nop)
	logger.SetGlobalLogger(nil)
	logger.SetGlobalLogger(new(logger.Logger{}))

	if logger.Global() == nil {
		t.Fatal("Global after set nil")
	}
}

func TestContextAndLog(t *testing.T) {
	t.Parallel()

	nop := new(logger.Logger{Logger: zap.NewNop()})

	var nilCtx context.Context

	if logger.Log(nilCtx) == nil {
		t.Fatal("Log(nil) nil")
	}

	ctx := logger.ContextWithLogger(nilCtx, nop)
	if logger.Log(ctx) == nil {
		t.Fatal("Log(ctx) nil")
	}

	ctx = logger.ContextWithLogger(context.Background(), nil)
	if ctx == nil {
		t.Fatal("ContextWithLogger nil lg")
	}

	_ = logger.ContextWithLogger(context.Background(), new(logger.Logger{}))
	_ = logger.Log(context.Background())
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	id := logger.NewRequestID()
	if id == "" {
		t.Fatal("empty request id")
	}

	var nilCtx context.Context

	ctx := logger.NewRequestIDContext(nilCtx, "req-1")
	if got := logger.RequestIDFromContext(ctx); got != "req-1" {
		t.Fatalf("got %q", got)
	}

	ctx = logger.NewRequestIDContext(context.Background(), "")
	if logger.RequestIDFromContext(ctx) == "" {
		t.Fatal("expected pid fallback id")
	}

	if logger.RequestIDFromContext(nilCtx) != "" {
		t.Fatal("nil ctx should be empty")
	}

	if logger.RequestIDFromContext(context.Background()) != "" {
		t.Fatal("missing key should be empty")
	}
}

func TestLoggerMethods(t *testing.T) {
	t.Parallel()

	lg := new(logger.Logger{Logger: zap.NewNop()})
	ctx := logger.NewRequestIDContext(context.Background(), "rid")

	var nilCtx context.Context

	lg.Info(ctx, "info")
	lg.Warn(ctx, "warn")
	lg.Error(ctx, "error")
	lg.Debug(ctx, "debug")
	lg.Info(nilCtx, "info-nil-ctx")
	lg.Info(context.Background(), "info-no-rid")

	with := lg.With(zap.String("k", "v"))
	if with == nil {
		t.Fatal("With nil")
	}

	var nilLg *logger.Logger

	nilLg.Info(ctx, "x")
	nilLg.Warn(ctx, "x")
	nilLg.Error(ctx, "x")
	nilLg.Debug(ctx, "x")

	if nilLg.With(zap.String("a", "b")) != nil {
		t.Fatal("nil With")
	}

	err := nilLg.Sync()
	if err != nil {
		t.Fatalf("nil Sync: %v", err)
	}

	empty := new(logger.Logger{})
	empty.Info(ctx, "x")
	empty.Warn(ctx, "x")
	empty.Error(ctx, "x")
	empty.Debug(ctx, "x")

	if empty.With() != empty {
		t.Fatal("empty With should return receiver")
	}

	err = empty.Sync()
	if err != nil {
		t.Fatalf("empty Sync: %v", err)
	}

	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	failing := zap.New(zapcore.NewCore(enc, zapcore.AddSync(errSyncWriter{}), zapcore.DebugLevel))
	failLg := new(logger.Logger{Logger: failing})

	err = failLg.Sync()
	if !errors.Is(err, errSyncBoom) {
		t.Fatalf("Sync err = %v", err)
	}

	_ = logger.Method(ctx, "M")
}
