package app

import (
	"context"
	"errors"
	"fmt"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"go.uber.org/zap"
)

type AuthServiceClient interface {
	Register(ctx context.Context, email, username, password string) (*authv1.RegisterResponse, error)
	Login(ctx context.Context, email, password string) (*authv1.LoginResponse, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*authv1.RefreshTokensResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	GetUserProfile(ctx context.Context) (*authv1.UserProfileResponse, error)
}

type AuthService struct {
	authClient AuthServiceClient
	cache      ports.Cache
	resilience *fault.ServiceResilience
}

type AuthUseCase struct {
	userRepo    ports.UserRepository
	tokenRepo   ports.TokenRepository
	passwordSvc ports.PasswordService
	tokenSvc    ports.TokenService
}

type UserUseCase struct {
	userRepo ports.UserRepository
}

func NewAuthUseCase(userRepo ports.UserRepository, tokenRepo ports.TokenRepository, passwordSvc ports.PasswordService, tokenSvc ports.TokenService) ports.AuthUseCase {
	return new(AuthUseCase{userRepo: userRepo, tokenRepo: tokenRepo, passwordSvc: passwordSvc, tokenSvc: tokenSvc})
}

func NewUserUseCase(userRepo ports.UserRepository) ports.UserUseCase {
	return new(UserUseCase{userRepo: userRepo})
}

func NewAuthService(authClient AuthServiceClient, cache ports.Cache) ports.AuthService {
	return new(AuthService{authClient: authClient, cache: cache, resilience: fault.NewServiceResilience("auth-service")})
}

func (a *AuthUseCase) Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error) {
	log := appLog(ctx, "AuthUseCase.Register").With(zap.String("email", email))
	log.Debug(ctx, "starting user registration")
	if err := validateEmail(email); err != nil {
		log.Debug(ctx, "invalid email format", zap.Error(err))
		return nil, wrapErr(ctx, domain.ErrCtxValidatingEmail, err)
	}
	if username == "" {
		log.Debug(ctx, "empty username provided")
		return nil, wrapErr(ctx, domain.ErrCtxValidatingUsername, domain.ErrEmptyUsername)
	}
	if err := validatePassword(password); err != nil {
		log.Debug(ctx, "invalid password", zap.Error(err))
		return nil, wrapErr(ctx, domain.ErrCtxValidatingPassword, err)
	}
	existingUser, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		log.Error(ctx, domain.ErrCtxCheckingUser.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxCheckingUser, err)
	}
	if existingUser != nil {
		log.Debug(ctx, "user with this email already exists")
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxEmailRegistered, domain.ErrEmailAlreadyExists)
	}
	hashedPassword, err := a.passwordSvc.Hash(ctx, password)
	if err != nil {
		log.Error(ctx, domain.ErrCtxHashingPassword.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxHashingPassword, err)
	}
	createdUser, err := a.userRepo.Create(ctx, new(domain.User{Email: email, Username: username, PasswordHash: hashedPassword}))
	if err != nil {
		log.Error(ctx, domain.ErrCtxCreatingUser.Error(), zap.Error(err))
		return nil, wrapErr(ctx, domain.ErrCtxCreatingUser, err)
	}
	log.Info(ctx, "user registered successfully", zap.String("userID", createdUser.ID))
	tokenPair, err := a.generateTokenPair(ctx, createdUser)
	if err != nil {
		log.Error(ctx, domain.ErrCtxGeneratingTokens.Error(), zap.Error(err), zap.String("userID", createdUser.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingTokens, err)
	}
	log.Info(ctx, "authentication tokens generated for new user", zap.String("userID", createdUser.ID))
	return tokenPair, nil
}

func (a *AuthUseCase) Login(ctx context.Context, email, password string) (*domain.TokenPair, error) {
	log := appLog(ctx, "AuthUseCase.Login").With(zap.String("email", email))
	log.Debug(ctx, "login attempt")
	user, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			log.Debug(ctx, "login attempt with non-existent email")
			return nil, fmt.Errorf("%s: %w", domain.ErrInvalidCredentials, domain.ErrInvalidCredentials)
		}
		log.Error(ctx, domain.ErrCtxFindingUser.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingUser, err)
	}
	valid, err := a.passwordSvc.Verify(ctx, password, user.PasswordHash)
	if err != nil {
		log.Error(ctx, domain.ErrCtxVerifyingPassword.Error(), zap.Error(err), zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxVerifyingPassword, err)
	}
	if !valid {
		log.Debug(ctx, "invalid password provided", zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidCredentials, domain.ErrInvalidCredentials)
	}
	log.Info(ctx, "user logged in successfully", zap.String("userID", user.ID))
	tokenPair, err := a.generateTokenPair(ctx, user)
	if err != nil {
		log.Error(ctx, domain.ErrCtxGeneratingTokens.Error(), zap.Error(err), zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingTokens, err)
	}
	log.Info(ctx, "authentication tokens generated for user", zap.String("userID", user.ID))
	return tokenPair, nil
}

func (a *AuthUseCase) RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(zap.String("method", "RefreshTokens"))
	log.Debug(ctx, "refreshing tokens")
	token, err := a.tokenRepo.FindByToken(ctx, refreshToken)
	if err != nil {
		log.Debug(ctx, domain.ErrInvalidRefreshToken.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingRefreshToken, domain.ErrInvalidRefreshToken)
	}
	log = log.With(zap.String("userID", token.UserID))
	if token.IsRevoked {
		log.Debug(ctx, "attempt to use revoked token")
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxTokenRevoked, domain.ErrRevokedRefreshToken)
	}
	user, err := a.userRepo.FindByID(ctx, token.UserID)
	if err != nil {
		log.Error(ctx, domain.ErrCtxFindingUser.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingUser, err)
	}
	if err := a.tokenRepo.RevokeToken(ctx, refreshToken); err != nil {
		log.Error(ctx, domain.ErrCtxRevokingOldToken.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxRevokingOldToken, err)
	}
	log.Debug(ctx, "old token revoked successfully")
	tokenPair, err := a.generateTokenPair(ctx, user)
	if err != nil {
		log.Error(ctx, domain.ErrCtxGeneratingNewTokens.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingNewTokens, err)
	}
	log.Info(ctx, "tokens refreshed successfully")
	return tokenPair, nil
}

func (a *AuthUseCase) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx).With(zap.String("method", "Logout"))
	log.Debug(ctx, "processing logout request")
	token, err := a.tokenRepo.FindByToken(ctx, refreshToken)
	if err == nil && token != nil {
		log = log.With(zap.String("userID", token.UserID))
	}
	err = a.tokenRepo.RevokeToken(ctx, refreshToken)
	if err != nil {
		log.Error(ctx, domain.ErrCtxRevokingToken.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrCtxRevokingToken, err)
	}
	log.Info(ctx, "user logged out successfully")
	return nil
}

func (a *AuthUseCase) generateTokenPair(ctx context.Context, user *domain.User) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(
		zap.String("method", "generateTokenPair"),
		zap.String("userID", user.ID),
	)
	accessToken, accessExpires, err := a.tokenSvc.GenerateAccessToken(ctx, user.ID, user.Username)
	if err != nil {
		log.Error(ctx, domain.ErrCtxGeneratingAccessToken.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingAccessToken, domain.ErrTokenGenerationFailed)
	}
	refreshToken, refreshExpires, err := a.tokenSvc.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		log.Error(ctx, domain.ErrCtxGeneratingRefreshToken.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingRefreshToken, domain.ErrTokenGenerationFailed)
	}
	if err := a.tokenRepo.StoreRefreshToken(ctx, new(domain.RefreshToken{
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: refreshExpires,
		IsRevoked: false,
	})); err != nil {
		log.Error(ctx, domain.ErrCtxStoringRefreshToken.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxStoringRefreshToken, err)
	}
	log.Debug(ctx, "token pair generated successfully")
	return new(domain.TokenPair{UserID: user.ID, Username: user.Username, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: accessExpires}), nil
}

func (u *UserUseCase) GetUserProfile(ctx context.Context, userID string) (*domain.User, error) {
	log := logger.Log(ctx).With(zap.String("method", "GetUserProfile"), zap.String("userID", userID))
	log.Debug(ctx, "requesting user profile")
	if userID == "" {
		log.Debug(ctx, "empty user ID provided")
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxValidatingUserID, domain.ErrEmptyUserID)
	}
	user, err := u.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Error(ctx, domain.ErrCtxFetchingProfile.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFetchingProfile, err)
	}
	log.Info(ctx, "user profile successfully retrieved")
	return user, nil
}
