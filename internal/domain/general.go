package domain

import (
	"time"
)

type Note struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
}

type User struct {
	CreatedAt, UpdatedAt              time.Time
	ID, Email, Username, PasswordHash string
}

type TokenPair struct {
	ExpiresAt time.Time
	//nolint:gosec
	UserID, Username, AccessToken, RefreshToken string
}

type RefreshToken struct {
	ExpiresAt, CreatedAt time.Time
	ID, UserID, Token    string
	IsRevoked            bool
}

type JWTConfig struct {
	SecretKey                       []byte
	AccessTokenTTL, RefreshTokenTTL time.Duration
}

type JWTClaims struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username,omitempty"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

const MinPasswordLength = 8
