package logger

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	RequestID   = "request_id"
	Development = "development"
	Production  = "production"
)

type (
	Logger     struct{ *zap.Logger }
	ctxKeyType string
)

const (
	requestIDCtxKey ctxKeyType = "request_id"
	loggerCtxKey    ctxKeyType = "logger_context"
)

//nolint:gochecknoglobals
var globalLogger atomic.Pointer[zap.Logger]

func buildFallbackLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)

	fallback, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}

	return fallback.Named("fallback")
}

func NewLogger(env, level string) (*Logger, error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		if env == Production {
			lvl = zapcore.InfoLevel
		} else {
			lvl = zapcore.DebugLevel
		}
	}

	cfg := zap.NewProductionConfig()
	if env != Production {
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.Level = zap.NewAtomicLevelAt(lvl)

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	return new(Logger{Logger: logger}), nil
}

func SetGlobalLogger(lg *Logger) {
	if nilChecking(lg) {
		return
	}

	globalLogger.Store(lg.Logger)
}

func Global() *Logger {
	if lg := globalLogger.Load(); lg != nil {
		return new(Logger{Logger: lg})
	}

	fallback := buildFallbackLogger()
	if globalLogger.CompareAndSwap(nil, fallback) {
		return new(Logger{Logger: fallback})
	}

	return new(Logger{Logger: globalLogger.Load()})
}

func Log(ctx context.Context) *Logger {
	if ctx == nil {
		return Global()
	}

	if l, ok := ctx.Value(loggerCtxKey).(*Logger); ok && l != nil {
		return l
	}

	return Global()
}

func ContextWithLogger(ctx context.Context, lg *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if nilChecking(lg) {
		return ctx
	}

	return context.WithValue(ctx, loggerCtxKey, lg)
}

func NewRequestID() string {
	return uuid.NewString()
}

func Method(ctx context.Context, method string) *Logger {
	return Log(ctx).With(zap.String("method", method))
}

func NewRequestIDContext(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if requestID == "" {
		requestID = strconv.Itoa(os.Getpid())
	}

	return context.WithValue(ctx, requestIDCtxKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if id, ok := ctx.Value(requestIDCtxKey).(string); ok {
		return id
	}

	return ""
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	if nilChecking(l) {
		return l
	}

	return new(Logger{Logger: l.Logger.With(fields...)})
}

func (l *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	if nilChecking(l) {
		return
	}

	l.Logger.Info(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	if nilChecking(l) {
		return
	}

	l.Logger.Warn(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	if nilChecking(l) {
		return
	}

	l.Logger.Error(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	if nilChecking(l) {
		return
	}

	l.Logger.Debug(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Sync() error {
	if nilChecking(l) {
		return nil
	}

	syncErr := l.Logger.Sync()
	if syncErr != nil {
		return fmt.Errorf("flush logger: %w", syncErr)
	}

	return nil
}

func addRequestID(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}

	if id := RequestIDFromContext(ctx); id != "" {
		return append(fields, zap.String(RequestID, id))
	}

	return fields
}

func nilChecking(l *Logger) bool {
	return l == nil || l.Logger == nil
}
