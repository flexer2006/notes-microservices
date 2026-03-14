package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

const (
	LogHandlerRegister        = "auth handler: register"
	LogHandlerLogin           = "auth handler: login"
	LogHandlerRefreshTokens   = "auth handler: refresh tokens" // #nosec G101
	LogHandlerLogout          = "auth handler: logout"
	LogHandlerGetProfile      = "auth handler: get profile"
	ErrorInvalidRequest       = "invalid request"
	ErrorFailedToServeRequest = "failed to serve request"
)

func sendErrorResponse(ctx fiber.Ctx, statusCode int, message string) error {
	if err := ctx.Status(statusCode).JSON(fiber.Map{
		"error": message,
	}); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

type AuthHandler struct {
	authService ports.AuthService
}

func NewAuthHandler(authService ports.AuthService) *AuthHandler {
	return new(AuthHandler{
		authService: authService,
	})
}

func (h *AuthHandler) Register(ctx fiber.Ctx) error {
	requestCtx := ctx.Context()
	log := logger.Log(requestCtx)
	log.Info(requestCtx, LogHandlerRegister)

	var req ports.RegisterRequest
	if err := ctx.Bind().JSON(&req); err != nil {
		log.Error(requestCtx, ErrorInvalidRequest, zap.Error(err))
		if err := ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": ErrorInvalidRequest,
		}); err != nil {
			return fmt.Errorf("error sending bad request response: %w", err)
		}
		return nil
	}
	if req.Email == "" || req.Username == "" || req.Password == "" {
		if err := ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "email, username and password are required",
		}); err != nil {
			return fmt.Errorf("error sending validation error response: %w", err)
		}
		return nil
	}
	response, err := h.authService.Register(requestCtx, &req)
	if err != nil {
		log.Error(requestCtx, ErrorFailedToServeRequest, zap.Error(err))
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "user already exists") {
			statusCode = http.StatusConflict
		}
		return sendErrorResponse(ctx, statusCode, err.Error())
	}
	if err := ctx.Status(http.StatusCreated).JSON(response); err != nil {
		return fmt.Errorf("sending response: %w", err)
	}
	return nil
}

func (h *AuthHandler) Login(ctx fiber.Ctx) error {
	requestCtx := ctx.Context()
	log := logger.Log(requestCtx)
	log.Info(requestCtx, LogHandlerLogin)
	var req ports.LoginRequest
	if err := ctx.Bind().JSON(&req); err != nil {
		log.Error(requestCtx, ErrorInvalidRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusBadRequest, ErrorInvalidRequest)
	}
	if req.Email == "" || req.Password == "" {
		return sendErrorResponse(ctx, http.StatusBadRequest, "email and password are required")
	}
	response, err := h.authService.Login(requestCtx, &req)
	if err != nil {
		log.Error(requestCtx, ErrorFailedToServeRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusUnauthorized, err.Error())
	}
	if err := ctx.Status(http.StatusOK).JSON(response); err != nil {
		return fmt.Errorf("sending response: %w", err)
	}
	return nil
}

func (h *AuthHandler) RefreshTokens(ctx fiber.Ctx) error {
	requestCtx := ctx.Context()
	log := logger.Log(requestCtx)
	log.Info(requestCtx, LogHandlerRefreshTokens)
	var req ports.RefreshRequest
	if err := ctx.Bind().JSON(&req); err != nil {
		log.Error(requestCtx, ErrorInvalidRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusBadRequest, ErrorInvalidRequest)
	}
	if req.RefreshToken == "" {
		return sendErrorResponse(ctx, http.StatusBadRequest, "refresh token is required")
	}
	response, err := h.authService.RefreshTokens(requestCtx, &req)
	if err != nil {
		log.Error(requestCtx, ErrorFailedToServeRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusUnauthorized, err.Error())
	}
	if err := ctx.Status(http.StatusOK).JSON(response); err != nil {
		return fmt.Errorf("sending response: %w", err)
	}
	return nil
}

func (h *AuthHandler) Logout(ctx fiber.Ctx) error {
	requestCtx := ctx.Context()
	log := logger.Log(requestCtx)
	log.Info(requestCtx, LogHandlerLogout)
	var req ports.LogoutRequest
	if err := ctx.Bind().JSON(&req); err != nil {
		log.Error(requestCtx, ErrorInvalidRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusBadRequest, ErrorInvalidRequest)
	}
	if req.RefreshToken == "" {
		return sendErrorResponse(ctx, http.StatusBadRequest, "refresh token is required")
	}
	err := h.authService.Logout(requestCtx, &req)
	if err != nil {
		log.Error(requestCtx, ErrorFailedToServeRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusInternalServerError, err.Error())
	}
	if err := ctx.Status(http.StatusOK).JSON(fiber.Map{
		"message": "logged out successfully",
	}); err != nil {
		return fmt.Errorf("sending response: %w", err)
	}
	return nil
}

func (h *AuthHandler) GetProfile(ctx fiber.Ctx) error {
	requestCtx := ctx.Context()
	log := logger.Log(requestCtx)
	log.Info(requestCtx, LogHandlerGetProfile)
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		return sendErrorResponse(ctx, http.StatusUnauthorized, "unauthorized")
	}
	profile, err := h.authService.GetUserProfile(userCtx)
	if err != nil {
		log.Error(requestCtx, ErrorFailedToServeRequest, zap.Error(err))
		return sendErrorResponse(ctx, http.StatusUnauthorized, err.Error())
	}
	if err := ctx.Status(http.StatusOK).JSON(profile); err != nil {
		return fmt.Errorf("sending response: %w", err)
	}
	return nil
}

const (
	LogHandlerCreateNote     = "handling create note request"
	LogHandlerGetNote        = "handling get note request"
	LogHandlerListNotes      = "handling list notes request"
	LogHandlerUpdateNote     = "handling update note request"
	LogHandlerDeleteNote     = "handling delete note request"
	ErrMsgInvalidNoteID      = "invalid note id"
	ErrMsgInvalidPagination  = "invalid pagination parameters"
	ErrMsgInvalidRequestBody = "invalid request body"
)

type Handler struct {
	notesService ports.NotesService
}

func NewNotesHandler(notesService ports.NotesService) *Handler {
	return new(Handler{
		notesService: notesService,
	})
}

func (h *Handler) CreateNote(ctx fiber.Ctx) error {
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		userCtx = ctx.Context()
	}
	log := logger.Log(userCtx).With(zap.String("handler", "Handler.CreateNote"))
	log.Debug(userCtx, LogHandlerCreateNote)
	var req ports.CreateNoteRequest
	if err := ctx.Bind().Body(&req); err != nil {
		log.Error(userCtx, ErrMsgInvalidRequestBody, zap.Error(err))
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidRequestBody,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	note, err := h.notesService.CreateNote(userCtx, &req)
	if err != nil {
		log.Error(userCtx, "failed to create note", zap.Error(err))
		return handleError(ctx, err)
	}
	if err := ctx.Status(fiber.StatusCreated).JSON(note); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

func (h *Handler) GetNote(ctx fiber.Ctx) error {
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		userCtx = ctx.Context()
	}
	log := logger.Log(userCtx).With(zap.String("handler", "Handler.GetNote"))
	log.Debug(userCtx, LogHandlerGetNote)
	noteID := ctx.Params("note_id")
	if noteID == "" {
		log.Error(userCtx, ErrMsgInvalidNoteID)
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidNoteID,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	note, err := h.notesService.GetNote(userCtx, noteID)
	if err != nil {
		log.Error(userCtx, "failed to get note", zap.Error(err))
		return handleError(ctx, err)
	}
	if err := ctx.JSON(note); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

func (h *Handler) ListNotes(ctx fiber.Ctx) error {
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		userCtx = ctx.Context()
	}
	log := logger.Log(userCtx).With(zap.String("handler", "Handler.ListNotes"))
	log.Debug(userCtx, LogHandlerListNotes)
	limitStr := ctx.Query("limit", "10")
	offsetStr := ctx.Query("offset", "0")
	limit, err := strconv.ParseInt(limitStr, 10, 32)
	if err != nil {
		log.Error(userCtx, ErrMsgInvalidPagination, zap.Error(err))
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidPagination,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	offset, err := strconv.ParseInt(offsetStr, 10, 32)
	if err != nil {
		log.Error(userCtx, ErrMsgInvalidPagination, zap.Error(err))
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidPagination,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	notes, err := h.notesService.ListNotes(userCtx, int32(limit), int32(offset))
	if err != nil {
		log.Error(userCtx, "failed to list notes", zap.Error(err))
		return handleError(ctx, err)
	}
	if err := ctx.JSON(notes); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

func (h *Handler) UpdateNote(ctx fiber.Ctx) error {
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		userCtx = ctx.Context()
	}
	log := logger.Log(userCtx).With(zap.String("handler", "Handler.UpdateNote"))
	log.Debug(userCtx, LogHandlerUpdateNote)
	noteID := ctx.Params("note_id")
	if noteID == "" {
		log.Error(userCtx, ErrMsgInvalidNoteID)
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidNoteID,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	var req ports.UpdateNoteRequest
	if err := ctx.Bind().Body(&req); err != nil {
		log.Error(userCtx, ErrMsgInvalidRequestBody, zap.Error(err))
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidRequestBody,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	note, err := h.notesService.UpdateNote(userCtx, noteID, &req)
	if err != nil {
		log.Error(userCtx, "failed to update note", zap.Error(err))
		return handleError(ctx, err)
	}
	if err := ctx.JSON(note); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

func (h *Handler) DeleteNote(ctx fiber.Ctx) error {
	userCtx, ok := ctx.Locals("userContext").(context.Context)
	if !ok {
		userCtx = ctx.Context()
	}
	log := logger.Log(userCtx).With(zap.String("handler", "Handler.DeleteNote"))
	log.Debug(userCtx, LogHandlerDeleteNote)
	noteID := ctx.Params("note_id")
	if noteID == "" {
		log.Error(userCtx, ErrMsgInvalidNoteID)
		if err := ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrMsgInvalidNoteID,
		}); err != nil {
			return fmt.Errorf("failed to send bad request response: %w", err)
		}
		return nil
	}
	err := h.notesService.DeleteNote(userCtx, noteID)
	if err != nil {
		log.Error(userCtx, "failed to delete note", zap.Error(err))
		return handleError(ctx, err)
	}
	if err := ctx.SendStatus(fiber.StatusNoContent); err != nil {
		return fmt.Errorf("error sending response: %w", err)
	}
	return nil
}

func handleError(ctx fiber.Ctx, err error) error {
	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		if err := ctx.Status(fiberErr.Code).JSON(fiber.Map{
			"error": fiberErr.Message,
		}); err != nil {
			return fmt.Errorf("fiber error response error: %w", err)
		}
		return nil
	}
	if err := ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error": "Internal server error",
	}); err != nil {
		return fmt.Errorf("error sending 500 response: %w", err)
	}
	return nil
}
