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

type UserHandler struct {
	tokenSvc    ports.TokenService
	userUseCase ports.UserUseCase
	authv1.UnimplementedUserServiceServer
}

func NewAuthHandler(authUseCase ports.AuthUseCase) *AuthHandler {
	return new(AuthHandler{authUseCase: authUseCase})
}

func NewUserHandler(userUseCase ports.UserUseCase, tokenSvc ports.TokenService) *UserHandler {
	return new(UserHandler{userUseCase: userUseCase, tokenSvc: tokenSvc})
}

func (h *AuthHandler) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	log := logger.Method(ctx, "Register")
	log.Info(ctx, "processing register request",
		zap.String("email", req.Email),
		zap.String("username", req.Username))
	if req.Email == "" || req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Register(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		log.Error(ctx, domain.ErrRegister.Error(), zap.Error(err))
		return nil, authErrorFromDomain(err)
	}
	return new(authv1.RegisterResponse{
		UserId:       tokenPair.UserID,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	log := logger.Method(ctx, "Login")
	log.Info(ctx, "processing login request", zap.String("email", req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidLoginParams.Error(), domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Login(ctx, req.Email, req.Password)
	if err != nil {
		log.Error(ctx, domain.ErrLogin.Error(), zap.Error(err))
		return nil, authErrorFromDomain(err)
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
	log := logger.Method(ctx, "RefreshTokens")
	log.Info(ctx, "processing refresh token request")
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrMissingRefreshToken.Error(), domain.ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, domain.ErrRefreshTokens.Error(), zap.Error(err))
		return nil, authErrorFromDomain(err)
	}
	return new(authv1.RefreshTokensResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Logout(ctx context.Context, req *authv1.LogoutRequest) (*emptypb.Empty, error) {
	log := logger.Method(ctx, "Logout")
	log.Info(ctx, "processing logout request")
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", domain.ErrMissingRefreshToken.Error(), domain.ErrInvalidRequest)
	}
	err := h.authUseCase.Logout(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, domain.ErrFailedToLogout.Error(), zap.Error(err))
		return nil, authErrorFromDomain(err)
	}
	return new(emptypb.Empty{}), nil
}

func (h *AuthHandler) RegisterService(server grpc.ServiceRegistrar) {
	authv1.RegisterAuthServiceServer(server, h)
}

func (h *UserHandler) GetUserProfile(ctx context.Context, _ *emptypb.Empty) (*authv1.UserProfileResponse, error) {
	log := logger.Method(ctx, "GetUserProfile")
	log.Info(ctx, "processing user profile request")
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.ErrMissingUserID.Error())
		return nil, fmt.Errorf("%s: %w", domain.ErrUnauthorized.Error(), domain.ErrMissingUserID)
	}
	userID, err := h.tokenSvc.ValidateAccessToken(ctx, token)
	if err != nil {
		log.Error(ctx, "invalid access token", zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrUnauthorized.Error(), domain.ErrMissingUserID)
	}
	user, err := h.userUseCase.GetUserProfile(ctx, userID)
	if err != nil {
		log.Error(ctx, domain.ErrGetUserProfile.Error(), zap.Error(err))
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, fmt.Errorf("%s: %w", domain.ErrProfileRetrievalFailed.Error(), domain.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", domain.ErrProfileRetrievalFailed.Error(), domain.ErrInternalService)
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
