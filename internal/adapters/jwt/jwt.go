package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

const (
	tokenUseAccess  = "access"
	tokenUseRefresh = "refresh"
)

type claims struct {
	jwt.RegisteredClaims

	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"`
	TokenUse string `json:"token_use"`
}

type Service struct {
	accessKey       []byte
	refreshKey      []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	parser          *jwt.Parser
}

func NewIssuer(
	accessSecret, refreshSecret string,
	accessTTL, refreshTTL time.Duration,
) *Service {
	return new(Service{
		accessKey:       []byte(accessSecret),
		refreshKey:      []byte(refreshSecret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		),
	})
}

func NewAccessVerifier(accessSecret string) *Service {
	return new(Service{
		accessKey:       []byte(accessSecret),
		refreshKey:      nil,
		accessTokenTTL:  0,
		refreshTokenTTL: 0,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		),
	})
}

func (s *Service) GenerateAccessToken(
	_ context.Context,
	userID, username string,
) (string, time.Time, error) {
	return s.generateToken(userID, username, s.accessTokenTTL, tokenUseAccess, s.accessKey)
}

func (s *Service) GenerateRefreshToken(
	_ context.Context,
	userID string,
) (string, time.Time, error) {
	return s.generateToken(userID, "", s.refreshTokenTTL, tokenUseRefresh, s.refreshKey)
}

func (s *Service) ValidateAccessToken(_ context.Context, tokenString string) (string, error) {
	tokenClaims := new(claims)

	token, err := s.parser.ParseWithClaims(
		tokenString,
		tokenClaims,
		func(_ *jwt.Token) (any, error) {
			if len(s.accessKey) == 0 {
				return nil, domain.ErrInvalidJWTToken
			}

			return s.accessKey, nil
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", domain.ErrExpiredJWTToken
		}

		return "", fmt.Errorf("%w: %w", domain.ErrInvalidJWTToken, err)
	}

	if !token.Valid || tokenClaims.UserID == "" || tokenClaims.TokenUse != tokenUseAccess {
		return "", domain.ErrInvalidJWTToken
	}

	return tokenClaims.UserID, nil
}

func (s *Service) generateToken(
	userID, username string,
	ttl time.Duration,
	tokenUse string,
	key []byte,
) (string, time.Time, error) {
	if len(key) == 0 {
		return "", time.Time{}, fmt.Errorf("%w: empty secret", domain.ErrTokenGeneration)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	tokenClaims := claims{
		UserID:   userID,
		Username: username,
		TokenUse: tokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)

	tokenString, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", domain.ErrTokenGeneration, err)
	}

	return tokenString, expiresAt, nil
}
