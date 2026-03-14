package http

import (
	"errors"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}
type TokenResponse struct {
	UserID       string    `json:"user_id"`
	Username     string    `json:"username,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type UserProfileResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
type AuthHandler struct {
	authService ports.AuthService
}

func NewAuthHandler(authService ports.AuthService) *AuthHandler {
	return new(AuthHandler{authService: authService})
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "auth.Register"))
	log.Info(ctx, domain.LogHandlerRegister)
	req, err := bindJSON[RegisterRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrorInvalidRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrorInvalidRequest)
	}
	if req.Email == "" || req.Username == "" || req.Password == "" {
		return errorResponse(c, fiber.StatusBadRequest, "email, username and password are required")
	}
	result, err := h.authService.Register(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToServeRequest, zap.Error(err))
		status := fiber.StatusInternalServerError
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			status = fiber.StatusConflict
		}
		return errorResponse(c, status, err.Error())
	}
	return jsonResponse(c, fiber.StatusCreated, TokenResponse{
		UserID:       result.UserID,
		Username:     result.Username,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	})
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "auth.Login"))
	log.Info(ctx, domain.LogHandlerLogin)
	req, err := bindJSON[LoginRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrorInvalidRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrorInvalidRequest)
	}
	if req.Email == "" || req.Password == "" {
		return errorResponse(c, fiber.StatusBadRequest, "email and password are required")
	}
	result, err := h.authService.Login(ctx, req.Email, req.Password)
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToServeRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	return jsonResponse(c, fiber.StatusOK, TokenResponse{
		UserID:       result.UserID,
		Username:     result.Username,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	})
}

func (h *AuthHandler) RefreshTokens(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "auth.RefreshTokens"))
	log.Info(ctx, domain.LogHandlerRefreshTokens)
	req, err := bindJSON[RefreshRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrorInvalidRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrorInvalidRequest)
	}
	if req.RefreshToken == "" {
		return errorResponse(c, fiber.StatusBadRequest, "refresh token is required")
	}
	result, err := h.authService.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToServeRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	return jsonResponse(c, fiber.StatusOK, TokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	})
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "auth.Logout"))
	log.Info(ctx, domain.LogHandlerLogout)
	req, err := bindJSON[LogoutRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrorInvalidRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrorInvalidRequest)
	}
	if req.RefreshToken == "" {
		return errorResponse(c, fiber.StatusBadRequest, "refresh token is required")
	}
	if err := h.authService.Logout(ctx, req.RefreshToken); err != nil {
		log.Error(ctx, domain.ErrorFailedToServeRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusInternalServerError, err.Error())
	}
	return jsonResponse(c, fiber.StatusOK, fiber.Map{"message": "logged out successfully"})
}

func (h *AuthHandler) GetProfile(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "auth.GetProfile"))
	log.Info(ctx, domain.LogHandlerGetProfile)
	profile, err := h.authService.GetUserProfile(ctx)
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToServeRequest, zap.Error(err))
		return errorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	return jsonResponse(c, fiber.StatusOK, UserProfileResponse{
		UserID:    profile.ID,
		Email:     profile.Email,
		Username:  profile.Username,
		CreatedAt: profile.CreatedAt})
}
