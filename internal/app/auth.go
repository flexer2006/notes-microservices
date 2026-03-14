package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"go.uber.org/zap"
)

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
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRegister), zap.String("email", email))
	log.Debug(ctx, domain.LogStartRegistration)
	if err := validateEmail(email); err != nil {
		log.Debug(ctx, domain.LogInvalidEmailFormat, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxValidatingEmail, err)
	}
	if username == "" {
		log.Debug(ctx, domain.LogEmptyUsernameProvided)
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxValidatingUsername, domain.ErrEmptyUsername)
	}
	if err := validatePassword(password); err != nil {
		log.Debug(ctx, domain.LogInvalidPassword, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxValidatingPassword, err)
	}
	existingUser, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		log.Error(ctx, domain.LogErrCheckExistingUser, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxCheckingUser, err)
	}
	if existingUser != nil {
		log.Debug(ctx, domain.LogEmailExists)
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxEmailRegistered, domain.ErrEmailAlreadyExists)
	}
	hashedPassword, err := a.passwordSvc.Hash(ctx, password)
	if err != nil {
		log.Error(ctx, domain.LogErrHashPassword, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxHashingPassword, err)
	}
	createdUser, err := a.userRepo.Create(ctx, new(domain.User{Email: email, Username: username, PasswordHash: hashedPassword}))
	if err != nil {
		log.Error(ctx, domain.LogErrCreateUser, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxCreatingUser, err)
	}
	log.Info(ctx, domain.LogUserRegistered, zap.String("userID", createdUser.ID))
	tokenPair, err := a.generateTokenPair(ctx, createdUser)
	if err != nil {
		log.Error(ctx, domain.LogErrGenerateTokens, zap.Error(err), zap.String("userID", createdUser.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingTokens, err)
	}
	log.Info(ctx, domain.LogTokensGenerated, zap.String("userID", createdUser.ID))
	return tokenPair, nil
}

func (a *AuthUseCase) Login(ctx context.Context, email, password string) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogin), zap.String("email", email))
	log.Debug(ctx, domain.LogLoginAttempt)
	user, err := a.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			log.Debug(ctx, domain.LogLoginNonExistent)
			return nil, fmt.Errorf("%s: %w", domain.ErrCtxInvalidCredentials, domain.ErrInvalidCredentials)
		}
		log.Error(ctx, domain.LogErrFindingUser, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingUser, err)
	}
	valid, err := a.passwordSvc.Verify(ctx, password, user.PasswordHash)
	if err != nil {
		log.Error(ctx, domain.LogErrVerifyingPassword, zap.Error(err), zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxVerifyingPassword, err)
	}
	if !valid {
		log.Debug(ctx, domain.LogInvalidPasswordAuth, zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxInvalidCredentials, domain.ErrInvalidCredentials)
	}
	log.Info(ctx, domain.LogUserLoggedIn, zap.String("userID", user.ID))
	tokenPair, err := a.generateTokenPair(ctx, user)
	if err != nil {
		log.Error(ctx, domain.LogErrGenerateLoginTokens, zap.Error(err), zap.String("userID", user.ID))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingTokens, err)
	}
	log.Info(ctx, domain.LogTokensGeneratedLogin, zap.String("userID", user.ID))
	return tokenPair, nil
}

func (a *AuthUseCase) RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRefreshTokens))
	log.Debug(ctx, domain.LogRefreshingTokens)
	token, err := a.tokenRepo.FindByToken(ctx, refreshToken)
	if err != nil {
		log.Debug(ctx, domain.LogErrInvalidRefreshToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingRefreshToken, domain.ErrInvalidRefreshToken)
	}
	log = log.With(zap.String("userID", token.UserID))
	if token.IsRevoked {
		log.Debug(ctx, domain.LogRevokedTokenAttempt)
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxTokenRevoked, domain.ErrRevokedRefreshToken)
	}
	user, err := a.userRepo.FindByID(ctx, token.UserID)
	if err != nil {
		log.Error(ctx, domain.LogErrFindingUserForToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFindingUser, err)
	}
	if err := a.tokenRepo.RevokeToken(ctx, refreshToken); err != nil {
		log.Error(ctx, domain.LogErrRevokingOldToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxRevokingOldToken, err)
	}
	log.Debug(ctx, domain.LogOldTokenRevoked)
	tokenPair, err := a.generateTokenPair(ctx, user)
	if err != nil {
		log.Error(ctx, domain.LogErrGenerateRefreshTokens, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingNewTokens, err)
	}
	log.Info(ctx, domain.LogTokensRefreshed)
	return tokenPair, nil
}

func (a *AuthUseCase) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogout))
	log.Debug(ctx, domain.LogProcessingLogout)
	token, err := a.tokenRepo.FindByToken(ctx, refreshToken)
	if err == nil && token != nil {
		log = log.With(zap.String("userID", token.UserID))
	}
	err = a.tokenRepo.RevokeToken(ctx, refreshToken)
	if err != nil {
		log.Error(ctx, domain.LogErrRevokingRefreshToken, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrCtxRevokingToken, err)
	}
	log.Info(ctx, domain.LogUserLoggedOut)
	return nil
}

func (a *AuthUseCase) generateTokenPair(ctx context.Context, user *domain.User) (*domain.TokenPair, error) {
	log := logger.Log(ctx).With(
		zap.String("method", domain.LogMethodGenerateTokenPair),
		zap.String("userID", user.ID),
	)
	accessToken, accessExpires, err := a.tokenSvc.GenerateAccessToken(ctx, user.ID, user.Username)
	if err != nil {
		log.Error(ctx, domain.LogErrGenerateAccessToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingAccessToken, domain.ErrTokenGenerationFailed)
	}
	refreshToken, refreshExpires, err := a.tokenSvc.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		log.Error(ctx, domain.LogErrGenerateRefreshToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxGeneratingRefreshToken, domain.ErrTokenGenerationFailed)
	}
	if err := a.tokenRepo.StoreRefreshToken(ctx, new(domain.RefreshToken{
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: refreshExpires,
		IsRevoked: false,
	})); err != nil {
		log.Error(ctx, domain.LogErrStoreRefreshToken, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxStoringRefreshToken, err)
	}
	log.Debug(ctx, domain.LogTokenPairGenerated)
	return new(domain.TokenPair{UserID: user.ID, Username: user.Username, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: accessExpires}), nil
}

func validateEmail(email string) error {
	if email == "" {
		return domain.ErrInvalidEmail
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`).MatchString(email) {
		return domain.ErrInvalidEmail
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return domain.ErrPasswordTooShort
	}
	if !regexp.MustCompile(`[a-zA-Z]`).MatchString(password) || !regexp.MustCompile(`\d`).MatchString(password) {
		return domain.ErrPasswordTooWeak
	}
	return nil
}

func GetValidatePasswordFunc() func(string) error {
	return validatePassword
}

func GetValidateEmailFunc() func(string) error {
	return validateEmail
}

func (a *AuthUseCase) GetGenerateTokenPairFunc() func(context.Context, *domain.User) (*domain.TokenPair, error) {
	return a.generateTokenPair
}

type UserUseCase struct {
	userRepo ports.UserRepository
}

func (u *UserUseCase) GetUserProfile(ctx context.Context, userID string) (*domain.User, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodGetUserProfile), zap.String("userID", userID))
	log.Debug(ctx, domain.LogRequestingProfile)
	if userID == "" {
		log.Debug(ctx, domain.LogEmptyUserIDProvided)
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxValidatingUserID, domain.ErrEmptyUserID)
	}
	user, err := u.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Error(ctx, domain.LogErrFindingUserByID, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrCtxFetchingProfile, err)
	}
	log.Info(ctx, domain.LogProfileRetrieved)
	return user, nil
}
