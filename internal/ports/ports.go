package ports

import (
	"context"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

type NoteRepository interface {
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Note, int, error)
	GetByID(ctx context.Context, noteID, userID string) (*domain.Note, error)
	Create(ctx context.Context, note *domain.Note) (string, error)
	Update(ctx context.Context, note *domain.Note) error
	Delete(ctx context.Context, noteID, userID string) error
}

type Cache interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Close() error
}

type AuthService interface {
	Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error)
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	GetUserProfile(ctx context.Context) (*domain.User, error)
}

type NotesService interface {
	CreateNote(ctx context.Context, title, content string) (*domain.Note, error)
	UpdateNote(ctx context.Context, noteID string, title, content *string) (*domain.Note, error)
	GetNote(ctx context.Context, noteID string) (*domain.Note, error)
	ListNotes(ctx context.Context, limit, offset int32) ([]*domain.Note, int, error)
	DeleteNote(ctx context.Context, noteID string) error
}

type AuthUseCase interface {
	Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error)
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type UserUseCase interface {
	GetUserProfile(ctx context.Context, userID string) (*domain.User, error)
}

type TokenRepository interface {
	StoreRefreshToken(ctx context.Context, token *domain.RefreshToken) error
	FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	RevokeToken(ctx context.Context, token string) error
	RevokeAllUserTokens(ctx context.Context, userID string) error
	CleanupExpiredTokens(ctx context.Context) error
	FindUserTokens(ctx context.Context, userID string) ([]*domain.RefreshToken, error)
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) (*domain.User, error)
	Delete(ctx context.Context, id string) error
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
