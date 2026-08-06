package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

// dummyPasswordHash equalizes login timing for unknown emails (bcrypt cost 10).
//
//nolint:gosec // not a live credential; fixed hash for Verify timing only
const dummyPasswordHash = "$2a$10$acL5avM/6CNzik0Jui7GuONl52bA9kU1xSbMCcBAnvaOSZ8hspht6"

type AuthUseCase struct {
	userRepo    ports.UserRepository
	tokenRepo   ports.TokenRepository
	passwordSvc ports.PasswordService
	tokenSvc    ports.TokenService
}

type UserUseCase struct {
	userRepo ports.UserRepository
}

func NewAuthUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	passwordSvc ports.PasswordService,
	tokenSvc ports.TokenService,
) *AuthUseCase {
	return new(
		AuthUseCase{
			userRepo:    userRepo,
			tokenRepo:   tokenRepo,
			passwordSvc: passwordSvc,
			tokenSvc:    tokenSvc,
		},
	)
}

func NewUserUseCase(userRepo ports.UserRepository) *UserUseCase {
	return new(UserUseCase{userRepo: userRepo})
}

func (a *AuthUseCase) Register(
	ctx context.Context,
	email, username, password string,
) (*domain.TokenPair, error) {
	log := appLog(ctx, "AuthUseCase.Register")

	emailErr := domain.ValidateEmail(email)
	if emailErr != nil {
		return nil, fmt.Errorf("register: %w", emailErr)
	}

	passwordErr := domain.ValidatePassword(password)
	if passwordErr != nil {
		return nil, fmt.Errorf("register: %w", passwordErr)
	}

	existingUser, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("register: checking existing user: %w", err)
	}

	if existingUser != nil {
		return nil, fmt.Errorf("register: %w", domain.ErrEmailAlreadyExists)
	}

	hashedPassword, err := a.passwordSvc.Hash(ctx, password)
	if err != nil {
		return nil, fmt.Errorf("register: hashing password: %w", err)
	}

	newUser, err := domain.NewUser(email, username, hashedPassword)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}

	createdUser, err := a.userRepo.Create(ctx, newUser)
	if err != nil {
		return nil, fmt.Errorf("register: creating user: %w", err)
	}

	log.Info(ctx, "user registered", zap.String("userID", createdUser.ID))

	tokenPair, err := a.generateTokenPair(ctx, createdUser)
	if err != nil {
		return nil, fmt.Errorf("register: generating tokens: %w", err)
	}

	return tokenPair, nil
}

func (a *AuthUseCase) Login(
	ctx context.Context,
	email, password string,
) (*domain.TokenPair, error) {
	log := appLog(ctx, "AuthUseCase.Login")

	user, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("login: finding user: %w", err)
	}

	hash := dummyPasswordHash
	if user != nil {
		hash = user.PasswordHash
	}

	valid, verifyErr := a.passwordSvc.Verify(ctx, password, hash)
	if verifyErr != nil {
		return nil, fmt.Errorf("login: verifying password: %w", verifyErr)
	}

	if user == nil || !valid {
		return nil, fmt.Errorf("login: %w", domain.ErrInvalidCredentials)
	}

	tokenPair, err := a.generateTokenPair(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("login: generating tokens: %w", err)
	}

	log.Info(ctx, "user logged in", zap.String("userID", user.ID))

	return tokenPair, nil
}

func (a *AuthUseCase) RefreshTokens(
	ctx context.Context,
	refreshToken string,
) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(zap.String("method", "AuthUseCase.RefreshTokens"))
	tokenHash := hashToken(refreshToken)

	token, err := a.tokenRepo.FindByToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRefreshToken) {
			return nil, fmt.Errorf("refresh: %w", domain.ErrInvalidRefreshToken)
		}

		return nil, fmt.Errorf("refresh: looking up token: %w", err)
	}

	log = log.With(zap.String("userID", token.UserID))
	if token.IsRevoked {
		return nil, a.rejectRefreshReuse(ctx, token.UserID)
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("refresh: %w", domain.ErrInvalidRefreshToken)
	}

	user, err := a.userRepo.FindByID(ctx, token.UserID)
	if err != nil {
		return nil, fmt.Errorf("refresh: finding user: %w", err)
	}

	accessToken, accessExpires, err := a.tokenSvc.GenerateAccessToken(ctx, user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("refresh: generating access token: %w", err)
	}

	newRefreshToken, refreshExpires, err := a.tokenSvc.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("refresh: generating refresh token: %w", err)
	}

	_, rotateErr := a.tokenRepo.RotateRefreshToken(ctx, tokenHash, new(domain.RefreshToken{
		ExpiresAt: refreshExpires,
		CreatedAt: time.Time{},
		ID:        "",
		UserID:    user.ID,
		Token:     hashToken(newRefreshToken),
		IsRevoked: false,
	}))
	if rotateErr != nil {
		if errors.Is(rotateErr, domain.ErrInvalidRefreshToken) {
			return nil, a.rejectRefreshReuse(ctx, token.UserID)
		}

		return nil, fmt.Errorf("refresh: rotating refresh token: %w", rotateErr)
	}

	log.Info(ctx, "tokens refreshed")

	return new(domain.TokenPair{
		ExpiresAt:    accessExpires,
		UserID:       user.ID,
		Username:     user.Username,
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}), nil
}

func (a *AuthUseCase) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx).With(zap.String("method", "AuthUseCase.Logout"))

	tokenHash := hashToken(refreshToken)

	storedToken, findErr := a.tokenRepo.FindByToken(ctx, tokenHash)
	if findErr == nil {
		log = log.With(zap.String("userID", storedToken.UserID))
	}

	err := a.tokenRepo.RevokeToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRefreshToken) {
			log.Info(ctx, "logout idempotent: token already absent or revoked")

			return nil
		}

		return fmt.Errorf("logout: revoking token: %w", err)
	}

	log.Info(ctx, "user logged out")

	return nil
}

func (a *AuthUseCase) rejectRefreshReuse(ctx context.Context, userID string) error {
	if userID != "" {
		revokeErr := a.tokenRepo.RevokeAllUserTokens(ctx, userID)
		if revokeErr != nil {
			return fmt.Errorf("refresh: revoking sessions after reuse: %w", revokeErr)
		}
	}

	return fmt.Errorf("refresh: %w", domain.ErrRevokedRefreshToken)
}

func (a *AuthUseCase) generateTokenPair(
	ctx context.Context,
	user *domain.User,
) (*domain.TokenPair, error) {
	accessToken, accessExpires, err := a.tokenSvc.GenerateAccessToken(ctx, user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, refreshExpires, err := a.tokenSvc.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	storeErr := a.tokenRepo.StoreRefreshToken(ctx, new(domain.RefreshToken{
		ExpiresAt: refreshExpires,
		CreatedAt: time.Time{},
		ID:        "",
		UserID:    user.ID,
		Token:     hashToken(refreshToken),
		IsRevoked: false,
	}))
	if storeErr != nil {
		return nil, fmt.Errorf("storing refresh token: %w", storeErr)
	}

	return new(domain.TokenPair{
		ExpiresAt:    accessExpires,
		UserID:       user.ID,
		Username:     user.Username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}), nil
}

func (u *UserUseCase) GetUserProfile(ctx context.Context, userID string) (*domain.User, error) {
	if userID == "" {
		return nil, fmt.Errorf("get profile: %w", domain.ErrEmptyUserID)
	}

	user, err := u.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	return user, nil
}
