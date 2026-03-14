package http

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

const (
	LogAuthMiddleware = "auth middleware"

	ErrorNoAuthHeader       = "no authorization header provided"
	ErrorInvalidTokenFormat = "invalid token format"
)

func NewAuthMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx).With(zap.String("middleware", "auth"))
		log.Debug(requestCtx, LogAuthMiddleware)
		authHeader := ctx.Get("Authorization")
		if authHeader == "" {
			log.Debug(requestCtx, ErrorNoAuthHeader)
			return fmt.Errorf("%s: %w", ErrorNoAuthHeader,
				ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": ErrorNoAuthHeader,
				}))
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Debug(requestCtx, ErrorInvalidTokenFormat)
			return fmt.Errorf("%s: %w", ErrorInvalidTokenFormat,
				ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": ErrorInvalidTokenFormat,
				}))
		}
		md := metadata.New(map[string]string{
			"authorization": authHeader,
		})
		newCtx := metadata.NewIncomingContext(requestCtx, md)
		ctx.Locals("userContext", newCtx)
		return ctx.Next()
	}
}

func NewLoggerMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		start := time.Now()
		path := ctx.Path()
		method := ctx.Method()
		log := logger.Log(requestCtx).With(
			zap.String("path", path),
			zap.String("method", method),
			zap.String("ip", ctx.IP()),
		)
		log.Info(requestCtx, "Request started")
		err := ctx.Next()
		latency := time.Since(start)
		status := ctx.Response().StatusCode()
		logFields := []zap.Field{
			zap.Int("status", status),
			zap.Duration("latency", latency),
		}
		if err != nil {
			log.Error(requestCtx, "Request failed", append(logFields, zap.Error(err))...)
			return fmt.Errorf("request processing error: %w", err)
		}
		log.Info(requestCtx, "Request completed", logFields...)
		return nil
	}
}

func NewRecoveryMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx)
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error(requestCtx, "Server panic",
					zap.String("error", fmt.Sprintf("%v", r)),
					zap.String("stack", string(stack)),
				)
				if err := ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal Server Error",
				}); err != nil {
					log.Error(requestCtx, "Failed to send error response after panic", zap.Error(err))
				}
			}
		}()
		return ctx.Next()
	}
}
