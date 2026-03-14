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

func NewAuthMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx).With(zap.String("middleware", "auth"))
		log.Debug(requestCtx, domain.LogAuthMiddleware)
		authHeader := ctx.Get("Authorization")
		if authHeader == "" {
			log.Debug(requestCtx, domain.ErrorNoAuthHeader)
			return fmt.Errorf("%s: %w", domain.ErrorNoAuthHeader, ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": domain.ErrorNoAuthHeader}))
		}
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok {
			log.Debug(requestCtx, domain.ErrorInvalidTokenFormat)
			if err := ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": domain.ErrorInvalidTokenFormat}); err != nil {
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
		log.Info(requestCtx, domain.LogRequestStarted)
		logFields := []zap.Field{zap.Int("status", ctx.Response().StatusCode()), zap.Duration("latency", time.Since(start))}
		if err := ctx.Next(); err != nil {
			log.Error(requestCtx, domain.LogRequestFailed, append(logFields, zap.Error(err))...)
			return fmt.Errorf("%s: %w", domain.ErrorRequestProcessingFailed, err)
		}
		log.Info(requestCtx, domain.LogRequestCompleted, logFields...)
		return nil
	}
}

func NewRecoveryMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		requestCtx := ctx.Context()
		log := logger.Log(requestCtx)
		defer func() {
			if r := recover(); r != nil {
				log.Error(requestCtx, domain.LogServerPanic, zap.String("error", fmt.Sprintf("%v", r)), zap.String("stack", string(debug.Stack())))
				if err := ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": domain.ErrorInternalServerError}); err != nil {
					log.Error(requestCtx, domain.LogFailedToSendPanicResponse, zap.Error(err))
				}
			}
		}()
		return ctx.Next()
	}
}
