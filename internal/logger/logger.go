package logger

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const RequestID = "request_id"

type Environment string

const (
	Development Environment = "development"
	Production  Environment = "production"
)

type Logger struct{ l *zap.Logger }

type ctxKeyType struct{}

var (
	ctxKey       = ctxKeyType{}
	globalLogger atomic.Value
)

func init() {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	zapLogger, _ := cfg.Build()
	globalLogger.Store(zapLogger.With(zap.String("logger", "fallback")))
}

func NewLogger(env Environment, level string) (*Logger, error) {
	zapLevel := parseLogLevel(level, env)
	config := zap.NewProductionConfig()
	if env != "production" {
		config = zap.NewDevelopmentConfig()
	}
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	zapLogger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	return &Logger{l: zapLogger}, nil
}

func parseLogLevel(level string, env Environment) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		if env == Production {
			return zapcore.InfoLevel
		}
		return zapcore.DebugLevel
	}
}

func SetGlobalLogger(l *Logger) {
	if l == nil || l.l == nil {
		return
	}
	globalLogger.Store(l.l)
}

func Global() *Logger {
	if v := globalLogger.Load(); v != nil {
		if l, ok := v.(*zap.Logger); ok && l != nil {
			return new(Logger{l: l})
		}
	}
	return new(Logger{l: zap.NewNop()})
}

func Log(ctx context.Context) *Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey).(*Logger); ok && l != nil {
			return l
		}
	}
	return Global()
}

func NewContext(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxKey, l)
}

func NewRequestIDContext(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = fmt.Sprintf("%d", os.Getpid())
	}
	return context.WithValue(ctx, RequestID, requestID)
}

func GetRequestID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(RequestID).(string)
	return id, ok
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{l: l.l.With(fields...)}
}

func (l *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	fields = addRequestIDFromContext(ctx, fields)
	l.l.Info(msg, fields...)
}

func (l *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	fields = addRequestIDFromContext(ctx, fields)
	l.l.Warn(msg, fields...)
}

func (l *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	fields = addRequestIDFromContext(ctx, fields)
	l.l.Error(msg, fields...)
}

func (l *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	fields = addRequestIDFromContext(ctx, fields)
	l.l.Debug(msg, fields...)
}

func (l *Logger) Sync() error {
	if err := l.l.Sync(); err != nil {
		return fmt.Errorf("failed to sync logger: %w", err)
	}
	return nil
}

func addRequestIDFromContext(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}
	if id, ok := ctx.Value(RequestID).(string); ok && id != "" {
		return append(fields, zap.String(RequestID, id))
	}
	return fields
}
