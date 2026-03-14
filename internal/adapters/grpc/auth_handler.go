package grpc

import (
	"context"
	"errors"
	"fmt"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthHandler struct {
	authUseCase ports.AuthUseCase
	authv1.UnimplementedAuthServiceServer
}

func NewAuthHandler(authUseCase ports.AuthUseCase) *AuthHandler {
	return new(AuthHandler{authUseCase: authUseCase})
}

func (h *AuthHandler) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRegister))
	log.Info(ctx, domain.LogRegisterRequest,
		zap.String("email", req.Email),
		zap.String("username", req.Username))
	if req.Email == "" || req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequestParamsMsg, domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Register(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", domain.ErrRegisterMsg, err))
		switch err.Error() {
		case domain.ErrUserAlreadyExistsMsg:
			return nil, fmt.Errorf("%s: %w", domain.ErrUserRegistrationFailedMsg, domain.ErrUserAlreadyExists)
		default:
			return nil, fmt.Errorf("%s: %w", domain.ErrAuthServiceErrorMsg, domain.ErrAuthServiceInternal)
		}
	}
	return new(authv1.RegisterResponse{
		UserId:       tokenPair.UserID,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogin))
	log.Info(ctx, domain.LogLoginRequest, zap.String("email", req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidLoginParamsMsg, domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Login(ctx, req.Email, req.Password)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", domain.ErrLoginMsg, err))
		return nil, fmt.Errorf("%s: %w", domain.ErrAuthenticationFailedMsg, domain.ErrInvalidCredentials)
	}
	return new(authv1.LoginResponse{
		UserId:       tokenPair.UserID,
		Username:     tokenPair.Username,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) RefreshTokens(ctx context.Context, req *authv1.RefreshTokensRequest) (*authv1.RefreshTokensResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRefreshTokens))
	log.Info(ctx, domain.LogRefreshTokenRequest)
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrMissingRefreshTokenMsg, domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", domain.ErrRefreshTokensMsg, err))
		return nil, fmt.Errorf("%s: %w", domain.ErrTokenRefreshFailedMsg, domain.ErrInvalidRefreshToken)
	}
	return new(authv1.RefreshTokensResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Logout(ctx context.Context, req *authv1.LogoutRequest) (*emptypb.Empty, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogout))
	log.Info(ctx, domain.LogLogoutRequest)
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrMissingRefreshLogoutMsg, domain.ErrInvalidRequest)
	}
	err := h.authUseCase.Logout(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", domain.ErrLogoutMsg, err))
		if errors.Is(err, domain.ErrInvalidRefreshToken) {
			return nil, fmt.Errorf("%s: %w", domain.ErrLogoutFailedTokenMsg, domain.ErrInvalidRefreshToken)
		}
		return nil, fmt.Errorf("%s: %w", domain.ErrLogoutOperationFailedMsg, domain.ErrAuthServiceInternal)
	}
	return new(emptypb.Empty{}), nil
}

func (h *AuthHandler) RegisterService(server grpc.ServiceRegistrar) {
	authv1.RegisterAuthServiceServer(server, h)
}

type UserHandler struct {
	tokenSvc    ports.TokenService
	userUseCase ports.UserUseCase
	authv1.UnimplementedUserServiceServer
}

func NewUserHandler(userUseCase ports.UserUseCase, tokenSvc ports.TokenService) *UserHandler {
	return new(UserHandler{userUseCase: userUseCase, tokenSvc: tokenSvc})
}

func (h *UserHandler) GetUserProfile(ctx context.Context, _ *emptypb.Empty) (*authv1.UserProfileResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodGetUserProfile))
	log.Info(ctx, domain.LogGetUserProfileRequest)
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.ErrMissingUserIDMsg)
		return nil, fmt.Errorf("%s: %w", domain.ErrUnauthorizedAccessMsg, domain.ErrMissingUserID)
	}
	userID, err := h.tokenSvc.ValidateAccessToken(ctx, token)
	if err != nil {
		log.Error(ctx, domain.LogInvalidAccessTokenMsg, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrUnauthorizedAccessMsg, domain.ErrMissingUserID)
	}
	user, err := h.userUseCase.GetUserProfile(ctx, userID)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", domain.ErrGetUserProfileMsg, err))
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, fmt.Errorf("%s: %w", domain.ErrProfileRetrievalFailedMsg, domain.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", domain.ErrProfileRetrievalFailedMsg, domain.ErrInternalService)
	}
	return new(authv1.UserProfileResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Username:  user.Username,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}), nil
}

func (h *UserHandler) RegisterService(server grpc.ServiceRegistrar) {
	authv1.RegisterUserServiceServer(server, h)
}
