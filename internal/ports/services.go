package ports

import (
	"context"
	"time"
)

type Cache interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	Close() error
}

type PasswordService interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password, hash string) (bool, error)
}

type TokenService interface {
	GenerateAccessToken(ctx context.Context, userID, username string) (string, time.Time, error)
	GenerateRefreshToken(ctx context.Context, userID string) (string, time.Time, error)
	ValidateAccessToken(ctx context.Context, token string) (string, error)
}
