package http

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

const requestIDHeader = "X-Request-ID"

func NewRequestIDMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		requestID := ctx.Get(requestIDHeader)
		if requestID == "" {
			requestID = logger.NewRequestID()
		}
		requestCtx = logger.NewRequestIDContext(requestCtx, requestID)
		ctx.SetContext(requestCtx)
		ctx.Set(requestIDHeader, requestID)
		return ctx.Next()
	}
}

func NewAuthMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx).With(zap.String("middleware", "auth"))
		log.Debug(requestCtx, "auth middleware")
		authHeader := ctx.Get("Authorization")
		if authHeader == "" {
			log.Debug(requestCtx, domain.ErrNoAuthHeader.Error())
			if err := ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": domain.ErrNoAuthHeader.Error()}); err != nil {
				return fmt.Errorf("%s: %w", domain.ErrNoAuthHeader.Error(), err)
			}
			return nil
		}
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok {
			log.Debug(requestCtx, domain.ErrInvalidTokenFormat.Error())
			if err := ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": domain.ErrInvalidTokenFormat.Error()}); err != nil {
				return err
			}
			return nil
		}
		md := metadata.New(map[string]string{"authorization": token})
		ctx.Locals(userContextKey, metadata.NewIncomingContext(requestCtx, md))
		return ctx.Next()
	}
}

func NewLoggerMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx, start := ctx.Context(), time.Now()
		log := logger.Log(requestCtx).With(zap.String("path", ctx.Path()), zap.String("method", ctx.Method()), zap.String("ip", ctx.IP()))
		log.Info(requestCtx, "Request started")
		logFields := []zap.Field{zap.Int("status", ctx.Response().StatusCode()), zap.Duration("latency", time.Since(start))}
		if err := ctx.Next(); err != nil {
			log.Error(requestCtx, "Request failed", append(logFields, zap.Error(err))...)
			return fmt.Errorf("%s: %w", domain.ErrRequestProcessingFailed.Error(), err)
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
				log.Error(requestCtx, "Server panic", zap.String("error", fmt.Sprintf("%v", r)), zap.String("stack", string(debug.Stack())))
				if err := ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": domain.ErrInternalServerError.Error()}); err != nil {
					log.Error(requestCtx, "Failed to send error response after panic", zap.Error(err))
				}
			}
		}()
		return ctx.Next()
	}
}
