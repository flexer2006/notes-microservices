package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

const (
	methodValidateToken        = "ValidateAccessToken"
	methodGenerateAccessToken  = "GenerateAccessToken"
	methodGenerateRefreshToken = "GenerateRefreshToken"
	msgValidatingToken         = "validating token"
	msgTokenValidated          = "token validated successfully"
	msgInvalidToken            = "invalid token format"
	msgTokenExpired            = "token has expired"
	msgErrParsingToken         = "error parsing token" //nolint:gosec
	msgGeneratingAccessToken   = "generating access token"
	msgGeneratingRefreshToken  = "generating refresh token"
	msgTokenGenerated          = "token generated successfully"
	errSigningToken            = "error signing token"
	errCtxValidating           = "validating token"
	errCtxGenerating           = "generating token"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type ServiceJWT struct {
	secretKey                       []byte
	accessTokenTTL, refreshTokenTTL time.Duration
}

func NewJWT(secretKey string, accessTTL, refreshTTL time.Duration) ports.TokenService {
	return new(ServiceJWT{
		secretKey:       []byte(secretKey),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	})
}

func (s *ServiceJWT) ValidateAccessToken(ctx context.Context, tokenString string) (string, error) {
	log := logger.Log(ctx).With(zap.String("method", methodValidateToken))
	log.Debug(ctx, msgValidatingToken)
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidAlgorithm, token.Header["alg"])
		}
		return s.secretKey, nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "token is expired") {
			log.Debug(ctx, msgTokenExpired)
			return "", fmt.Errorf("%s: %w", errCtxValidating, domain.ErrExpiredJWTToken)
		}
		log.Error(ctx, msgErrParsingToken, zap.Error(err))
		return "", fmt.Errorf("%s: %w", errCtxValidating, domain.ErrInvalidJWTToken)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		log.Debug(ctx, msgInvalidToken)
		return "", fmt.Errorf("%s: %w", errCtxValidating, domain.ErrInvalidJWTToken)
	}
	if claims.UserID == "" {
		log.Debug(ctx, "user_id claim is empty")
		return "", fmt.Errorf("%s: %w", errCtxValidating, domain.ErrInvalidJWTToken)
	}
	log.Debug(ctx, msgTokenValidated, zap.String("userID", claims.UserID))
	return claims.UserID, nil
}

type ClaimsJwt = Claims

func domainToJWTClaims(claims domain.JWTClaims) Claims {
	return Claims{
		UserID:   claims.UserID,
		Username: claims.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(claims.IssuedAt),
			Subject:   claims.UserID,
		},
	}
}

func GetDomainToJWTClaimsForTest(claims domain.JWTClaims) Claims {
	return domainToJWTClaims(claims)
}

func jwtToDomainClaims(claims Claims) domain.JWTClaims {
	var expiresAt, issuedAt time.Time
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	return domain.JWTClaims{
		UserID:    claims.UserID,
		Username:  claims.Username,
		ExpiresAt: expiresAt,
		IssuedAt:  issuedAt,
	}
}

func GetJWTToDomainClaimsForTest(claims Claims) domain.JWTClaims {
	return jwtToDomainClaims(claims)
}

func (s *ServiceJWT) GenerateAccessToken(ctx context.Context, userID, username string) (string, time.Time, error) {
	log := logger.Log(ctx).With(
		zap.String("method", methodGenerateAccessToken),
		zap.String("userID", userID),
	)
	log.Debug(ctx, msgGeneratingAccessToken)
	if len(s.secretKey) == 0 {
		log.Error(ctx, "empty secret key provided")
		return "", time.Time{}, fmt.Errorf("%s: %w: empty secret key", errCtxGenerating, domain.ErrGeneratingJWTToken)
	}
	now := time.Now()
	expiresAt := now.Add(s.accessTokenTTL)
	domainClaims := domain.JWTClaims{
		UserID:    userID,
		Username:  username,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	jwtClaims := domainToJWTClaims(domainClaims)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		log.Error(ctx, errSigningToken, zap.Error(err))
		return "", time.Time{}, fmt.Errorf("%s: %w: %w", errCtxGenerating, domain.ErrGeneratingJWTToken, err)
	}
	log.Debug(ctx, msgTokenGenerated, zap.Time("expiresAt", expiresAt))
	return tokenString, expiresAt, nil
}

func (s *ServiceJWT) GenerateRefreshToken(ctx context.Context, userID string) (string, time.Time, error) {
	log := logger.Log(ctx).With(
		zap.String("method", methodGenerateRefreshToken),
		zap.String("userID", userID),
	)
	log.Debug(ctx, msgGeneratingRefreshToken)
	if len(s.secretKey) == 0 {
		log.Error(ctx, "empty secret key provided")
		return "", time.Time{}, fmt.Errorf("%s: %w: empty secret key", errCtxGenerating, domain.ErrGeneratingJWTToken)
	}
	now := time.Now()
	expiresAt := now.Add(s.refreshTokenTTL)
	domainClaims := domain.JWTClaims{
		UserID:    userID,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	jwtClaims := domainToJWTClaims(domainClaims)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		log.Error(ctx, errSigningToken, zap.Error(err))
		return "", time.Time{}, fmt.Errorf("%s: %w: %w", errCtxGenerating, domain.ErrGeneratingJWTToken, err)
	}
	log.Debug(ctx, msgTokenGenerated, zap.Time("expiresAt", expiresAt))
	return tokenString, expiresAt, nil
}
