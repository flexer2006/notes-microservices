package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

type AuthHandler struct {
	authv1.UnimplementedAuthServiceServer

	authUseCase ports.AuthUseCase
}

type UserHandler struct {
	authv1.UnimplementedUserServiceServer

	userUseCase ports.UserUseCase
}

func NewAuthHandler(authUseCase ports.AuthUseCase) *AuthHandler {
	return new(AuthHandler{
		UnimplementedAuthServiceServer: authv1.UnimplementedAuthServiceServer{},
		authUseCase:                    authUseCase,
	})
}

func NewUserHandler(userUseCase ports.UserUseCase) *UserHandler {
	return new(UserHandler{
		UnimplementedUserServiceServer: authv1.UnimplementedUserServiceServer{},
		userUseCase:                    userUseCase,
	})
}

func (h *AuthHandler) Register(
	ctx context.Context,
	req *authv1.RegisterRequest,
) (*authv1.RegisterResponse, error) {
	if req.GetEmail() == "" || req.GetUsername() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "email, username and password are required")
	}

	tokenPair, err := h.authUseCase.Register(
		ctx,
		req.GetEmail(),
		req.GetUsername(),
		req.GetPassword(),
	)
	if err != nil {
		return nil, authStatusFromError(err)
	}

	return new(authv1.RegisterResponse{
		UserId:       tokenPair.UserID,
		Username:     tokenPair.Username,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Login(
	ctx context.Context,
	req *authv1.LoginRequest,
) (*authv1.LoginResponse, error) {
	if req.GetEmail() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	tokenPair, err := h.authUseCase.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, authStatusFromError(err)
	}

	return new(authv1.LoginResponse{
		UserId:       tokenPair.UserID,
		Username:     tokenPair.Username,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) RefreshTokens(
	ctx context.Context,
	req *authv1.RefreshTokensRequest,
) (*authv1.RefreshTokensResponse, error) {
	if req.GetRefreshToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	tokenPair, err := h.authUseCase.RefreshTokens(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, authStatusFromError(err)
	}

	return new(authv1.RefreshTokensResponse{
		UserId:       tokenPair.UserID,
		Username:     tokenPair.Username,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Logout(
	ctx context.Context,
	req *authv1.LogoutRequest,
) (*emptypb.Empty, error) {
	if req.GetRefreshToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	logoutErr := h.authUseCase.Logout(ctx, req.GetRefreshToken())
	if logoutErr != nil {
		return nil, authStatusFromError(logoutErr)
	}

	return new(emptypb.Empty), nil
}

func (h *AuthHandler) RegisterService(server grpc.ServiceRegistrar) {
	authv1.RegisterAuthServiceServer(server, h)
}

func (h *UserHandler) GetUserProfile(
	ctx context.Context,
	_ *emptypb.Empty,
) (*authv1.UserProfileResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user, err := h.userUseCase.GetUserProfile(ctx, userID)
	if err != nil {
		return nil, authStatusFromError(err)
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
