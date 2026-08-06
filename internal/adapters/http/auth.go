package http

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

type RegisterRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type TokenResponse struct {
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type UserProfileResponse struct {
	CreatedAt time.Time `json:"created_at"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
}

type AuthHandler struct {
	authService ports.AuthService
}

func NewAuthHandler(authService ports.AuthService) *AuthHandler {
	return new(AuthHandler{authService: authService})
}

func (h *AuthHandler) Register(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	req, err := bindJSON[RegisterRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Email == "" || req.Username == "" || req.Password == "" {
		return errorResponse(
			fctx,
			fiber.StatusBadRequest,
			"email, username and password are required",
		)
	}

	emailErr := domain.ValidateEmail(req.Email)
	if emailErr != nil {
		return handleError(fctx, emailErr)
	}

	passwordErr := domain.ValidatePassword(req.Password)
	if passwordErr != nil {
		return handleError(fctx, passwordErr)
	}

	result, err := h.authService.Register(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusCreated, TokenResponse{
		UserID:       result.UserID,
		Username:     result.Username,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	})
}

func (h *AuthHandler) Login(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	req, err := bindJSON[LoginRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Email == "" || req.Password == "" {
		return errorResponse(fctx, fiber.StatusBadRequest, "email and password are required")
	}

	emailErr := domain.ValidateEmail(req.Email)
	if emailErr != nil {
		return handleError(fctx, emailErr)
	}

	result, err := h.authService.Login(ctx, req.Email, req.Password)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, TokenResponse{
		UserID:       result.UserID,
		Username:     result.Username,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	})
}

func (h *AuthHandler) RefreshTokens(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	req, err := bindJSON[RefreshRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if req.RefreshToken == "" {
		return errorResponse(fctx, fiber.StatusBadRequest, "refresh token is required")
	}

	result, err := h.authService.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, TokenResponse{
		ExpiresAt:    result.ExpiresAt,
		UserID:       result.UserID,
		Username:     result.Username,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
	})
}

func (h *AuthHandler) Logout(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	req, err := bindJSON[LogoutRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if req.RefreshToken == "" {
		return errorResponse(fctx, fiber.StatusBadRequest, "refresh token is required")
	}

	logoutErr := h.authService.Logout(ctx, req.RefreshToken)
	if logoutErr != nil {
		return handleError(fctx, logoutErr)
	}

	return jsonResponse(fctx, fiber.StatusOK, fiber.Map{"message": "logged out successfully"})
}

func (h *AuthHandler) GetProfile(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token := authctx.BearerTokenFrom(ctx)
	if token == "" {
		return errorResponse(fctx, fiber.StatusUnauthorized, errNoAuthHeader.Error())
	}

	profile, err := h.authService.GetUserProfile(ctx, token)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, UserProfileResponse{
		UserID:    profile.ID,
		Email:     profile.Email,
		Username:  profile.Username,
		CreatedAt: profile.CreatedAt,
	})
}
