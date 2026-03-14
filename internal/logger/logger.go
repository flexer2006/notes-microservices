package logger

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/flexer2006/notes-microservices/internal/domain"
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
	ctxKeyType struct{}
)

var (
	ctxKey       = ctxKeyType{}
	globalLogger atomic.Pointer[zap.Logger]
)

func init() {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	logger, err := cfg.Build()
	if err != nil {
		globalLogger.Store(zap.NewNop())
		return
	}
	globalLogger.Store(logger.Named("fallback"))
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
		return nil, fmt.Errorf("%s %w", domain.LogErrInitLoggerWithColon, err)
	}
	return new(Logger{Logger: logger}), nil
}

func SetGlobalLogger(l *Logger) {
	if check(l) {
		return
	}
	globalLogger.Store(l.Logger)
}

func Global() *Logger {
	if l := globalLogger.Load(); l != nil {
		return new(Logger{Logger: l})
	}
	return new(Logger{Logger: zap.NewNop()})
}

func Log(ctx context.Context) *Logger {
	if ctx == nil {
		return Global()
	}
	if l, ok := ctx.Value(ctxKey).(*Logger); ok && l != nil {
		return l
	}
	return Global()
}

func NewRequestIDContext(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = strconv.Itoa(os.Getpid())
	}
	return context.WithValue(ctx, RequestID, requestID)
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	if check(l) {
		return l
	}
	return new(Logger{Logger: l.Logger.With(fields...)})
}

func (l *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	if check(l) {
		return
	}
	l.Logger.Info(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	if check(l) {
		return
	}
	l.Logger.Warn(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	if check(l) {
		return
	}
	l.Logger.Error(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	if check(l) {
		return
	}
	l.Logger.Debug(msg, addRequestID(ctx, fields)...)
}

func (l *Logger) Sync() error {
	if check(l) {
		return nil
	}
	return l.Logger.Sync()
}

func addRequestID(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}
	if id, ok := ctx.Value(RequestID).(string); ok && id != "" {
		return append(fields, zap.String(RequestID, id))
	}
	return fields
}

func check(l *Logger) bool {
	return l == nil || l.Logger == nil
}
