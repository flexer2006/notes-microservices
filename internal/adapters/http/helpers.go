package http

import (
	"context"
	"errors"
	"strconv"

	"github.com/flexer2006/notes-microservices/internal/domain"

	"github.com/gofiber/fiber/v3"
)

const userContextKey = "userContext"

func userCtx(c fiber.Ctx) context.Context {
	if v, ok := c.Locals(userContextKey).(context.Context); ok {
		return v
	}
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func bindJSON[T any](c fiber.Ctx) (T, error) {
	var dst T
	return dst, c.Bind().JSON(&dst)
}

func jsonResponse(c fiber.Ctx, status int, body any) error {
	return c.Status(status).JSON(body)
}

func httpErrorFromDomain(err error) (int, string, bool) {
	if err == nil {
		return 0, "", false
	}
	if errors.Is(err, domain.ErrInvalidRequest) || errors.Is(err, domain.ErrInvalidRequestBody) || errors.Is(err, domain.ErrInvalidNoteID) {
		return fiber.StatusBadRequest, err.Error(), true
	}
	if errors.Is(err, domain.ErrUnauthorized) || errors.Is(err, domain.ErrInvalidTokenFormat) || errors.Is(err, domain.ErrNoAuthHeader) {
		return fiber.StatusUnauthorized, err.Error(), true
	}
	if errors.Is(err, domain.ErrUserAlreadyExists) {
		return fiber.StatusConflict, err.Error(), true
	}
	if errors.Is(err, domain.ErrNoteNotFound) {
		return fiber.StatusNotFound, err.Error(), true
	}
	return fiber.StatusInternalServerError, err.Error(), true
}

//nolint:unparam
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
	if status, msg, ok := httpErrorFromDomain(err); ok {
		return jsonResponse(c, status, fiber.Map{"error": msg})
	}
	return jsonResponse(c, fiber.StatusInternalServerError, fiber.Map{"error": "Internal server error"})
}

func noteToAPI(note *domain.Note) *Note {
	if note == nil {
		return nil
	}
	return new(Note{
		ID:        note.ID,
		UserID:    note.UserID,
		Title:     note.Title,
		Content:   note.Content,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
	})
}
