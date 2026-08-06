package http

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
)

const (
	errorKey               = "error"
	msgInternalServerError = "internal server error"
	statusKey              = "status"
)

func userCtx(c fiber.Ctx) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}

	return context.Background()
}

//nolint:ireturn // generic binder: T is the caller's concrete request type.
func bindJSON[T any](c fiber.Ctx) (T, error) {
	var dst T

	bindErr := c.Bind().JSON(&dst)
	if bindErr != nil {
		return dst, fmt.Errorf("bind json body: %w", bindErr)
	}

	return dst, nil
}

func jsonResponse(c fiber.Ctx, status int, body any) error {
	writeErr := c.Status(status).JSON(body)
	if writeErr != nil {
		return fmt.Errorf("write json response: %w", writeErr)
	}

	return nil
}

func httpErrorFromDomain(err error) (int, string, bool) {
	switch {
	case err == nil:
		return 0, "", false
	case errors.Is(err, domain.ErrInvalidParams),
		errors.Is(err, domain.ErrInvalidEmail),
		errors.Is(err, domain.ErrEmptyUsername),
		errors.Is(err, domain.ErrUsernameTooLong),
		errors.Is(err, domain.ErrEmptyUserID),
		errors.Is(err, domain.ErrPasswordTooShort),
		errors.Is(err, domain.ErrPasswordTooWeak),
		errors.Is(err, domain.ErrEmptyNoteTitle),
		errors.Is(err, domain.ErrNoteTitleTooLong),
		errors.Is(err, domain.ErrNoteContentTooLarge):
		return fiber.StatusBadRequest, err.Error(), true
	case errors.Is(err, domain.ErrUnauthorized),
		errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrInvalidRefreshToken),
		errors.Is(err, domain.ErrRevokedRefreshToken),
		errors.Is(err, domain.ErrInvalidJWTToken),
		errors.Is(err, domain.ErrExpiredJWTToken):
		return fiber.StatusUnauthorized, err.Error(), true
	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrNoteNotFound),
		errors.Is(err, domain.ErrNoteNotFoundOrNotOwned):
		return fiber.StatusNotFound, err.Error(), true
	case errors.Is(err, domain.ErrEmailAlreadyExists),
		errors.Is(err, domain.ErrUserAlreadyExists):
		return fiber.StatusConflict, err.Error(), true
	case errors.Is(err, domain.ErrServiceUnavailable),
		errors.Is(err, fault.ErrCircuitOpen):
		return fiber.StatusServiceUnavailable, "service temporarily unavailable", true
	default:
		return fiber.StatusInternalServerError, msgInternalServerError, true
	}
}

func errorResponse(c fiber.Ctx, status int, msg string) error {
	return jsonResponse(c, status, fiber.Map{errorKey: msg})
}

func queryInt(c fiber.Ctx, key string, def int) int {
	raw := c.Query(key, "")
	if raw == "" {
		return def
	}

	num, err := strconv.Atoi(raw)
	if err != nil || num < 0 {
		return def
	}

	return num
}

func handleError(fctx fiber.Ctx, err error) error {
	if ferr, ok := errors.AsType[*fiber.Error](err); ok {
		return jsonResponse(fctx, ferr.Code, fiber.Map{errorKey: ferr.Message})
	}

	if status, msg, ok := httpErrorFromDomain(err); ok {
		return jsonResponse(fctx, status, fiber.Map{errorKey: msg})
	}

	return jsonResponse(
		fctx,
		fiber.StatusInternalServerError,
		fiber.Map{errorKey: msgInternalServerError},
	)
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
