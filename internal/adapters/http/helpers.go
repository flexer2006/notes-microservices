package http

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
)

const userContextKey = "userContext"

func userCtx(c fiber.Ctx) context.Context {
	if v, ok := c.Locals(userContextKey).(context.Context); ok {
		return v
	}
	return c.Context()
}

func bindJSON[T any](c fiber.Ctx) (T, error) {
	var dst T
	return dst, c.Bind().JSON(&dst)
}

func jsonResponse(c fiber.Ctx, status int, body any) error {
	return c.Status(status).JSON(body)
}

func errorResponse(c fiber.Ctx, status int, msg string) error {
	return jsonResponse(c, status, fiber.Map{"error": msg})
}

func queryInt(c fiber.Ctx, key string, def int) int {
	if v := c.Query(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func handleError(c fiber.Ctx, err error) error {
	if ferr, ok := errors.AsType[*fiber.Error](err); ok {
		return jsonResponse(c, ferr.Code, fiber.Map{"error": ferr.Message})
	}
	return jsonResponse(c, fiber.StatusInternalServerError, fiber.Map{"error": "Internal server error"})
}
