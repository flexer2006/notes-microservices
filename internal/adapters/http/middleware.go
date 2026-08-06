package http

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

const requestIDHeader = "X-Request-ID"

var (
	errNoAuthHeader      = errors.New("no authorization header provided")
	errInvalidAuthFormat = errors.New("invalid authorization header format")
)

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

		authHeader := ctx.Get("Authorization")
		if authHeader == "" {
			return ctx.Status(fiber.StatusUnauthorized).
				JSON(fiber.Map{errorKey: errNoAuthHeader.Error()})
		}

		scheme, token, ok := strings.Cut(authHeader, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			return ctx.Status(fiber.StatusUnauthorized).
				JSON(fiber.Map{errorKey: errInvalidAuthFormat.Error()})
		}

		ctx.SetContext(authctx.WithBearerToken(requestCtx, strings.TrimSpace(token)))

		return ctx.Next()
	}
}

func NewLoggerMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx, start := ctx.Context(), time.Now()
		log := logger.Log(requestCtx).With(
			zap.String("path", ctx.Path()),
			zap.String("method", ctx.Method()),
			zap.String("ip", ctx.IP()),
		)
		err := ctx.Next()

		fields := []zap.Field{
			zap.Int("status", ctx.Response().StatusCode()),
			zap.Duration("latency", time.Since(start)),
		}
		if err != nil {
			log.Error(requestCtx, "Request failed", append(fields, zap.Error(err))...)

			return fmt.Errorf("request processing: %w", err)
		}

		log.Info(requestCtx, "Request completed", fields...)

		return nil
	}
}

func NewRecoveryMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx)

		defer func() {
			if r := recover(); r != nil {
				log.Error(
					requestCtx,
					"Server panic",
					zap.String("error", fmt.Sprintf("%v", r)),
					zap.String("stack", string(debug.Stack())),
				)

				err := ctx.Status(fiber.StatusInternalServerError).
					JSON(fiber.Map{errorKey: msgInternalServerError})
				if err != nil {
					log.Error(
						requestCtx,
						"Failed to send error response after panic",
						zap.Error(err),
					)
				}
			}
		}()

		return ctx.Next()
	}
}
