package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

type claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"`
}

type Service struct {
	key                             []byte
	accessTokenTTL, refreshTokenTTL time.Duration
	parser                          *jwt.Parser
}

func New(secret string, accessTTL, refreshTTL time.Duration) ports.TokenService {
	return new(Service{key: []byte(secret), accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL, parser: jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))})
}

func (s *Service) GenerateAccessToken(_ context.Context, userID, username string) (string, time.Time, error) {
	return s.generateToken(userID, username, s.accessTokenTTL)
}

func (s *Service) GenerateRefreshToken(_ context.Context, userID string) (string, time.Time, error) {
	return s.generateToken(userID, "", s.refreshTokenTTL)
}

func (s *Service) generateToken(userID, username string, ttl time.Duration) (string, time.Time, error) {
	if len(s.key) == 0 {
		return "", time.Time{}, fmt.Errorf("%w: empty secret", domain.ErrGeneratingJWTToken)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	claims := claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", domain.ErrGeneratingJWTToken, err)
	}
	return tokenString, expiresAt, nil
}

func (s *Service) ValidateAccessToken(_ context.Context, tokenString string) (string, error) {
	claims := new(claims)
	token, err := s.parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return s.key, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", domain.ErrExpiredJWTToken
		}
		return "", fmt.Errorf("%w: %v", domain.ErrInvalidJWTToken, err)
	}
	if !token.Valid || claims.UserID == "" {
		return "", domain.ErrInvalidJWTToken
	}
	return claims.UserID, nil
}
