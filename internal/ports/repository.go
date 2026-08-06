package ports

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

type NoteRepository interface {
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Note, int, error)
	GetByID(ctx context.Context, noteID, userID string) (*domain.Note, error)
	Create(ctx context.Context, note *domain.Note) (*domain.Note, error)
	Update(ctx context.Context, note *domain.Note) error
	Delete(ctx context.Context, noteID, userID string) error
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) (*domain.User, error)
	Delete(ctx context.Context, id string) error
}

type TokenRepository interface {
	StoreRefreshToken(ctx context.Context, token *domain.RefreshToken) error
	FindByToken(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	RevokeToken(ctx context.Context, tokenHash string) error
	ConsumeActiveToken(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	RotateRefreshToken(
		ctx context.Context,
		oldHash string,
		newToken *domain.RefreshToken,
	) (*domain.RefreshToken, error)
	RevokeAllUserTokens(ctx context.Context, userID string) error
	CleanupExpiredTokens(ctx context.Context) error
	FindUserTokens(ctx context.Context, userID string) ([]*domain.RefreshToken, error)
}
