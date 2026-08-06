package ports

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

type AuthUseCase interface {
	Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error)
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type UserUseCase interface {
	GetUserProfile(ctx context.Context, userID string) (*domain.User, error)
}

// NoteUseCase
//
//nolint:iface
type NoteUseCase interface {
	CreateNote(ctx context.Context, userID, title, content string) (*domain.Note, error)
	GetNote(ctx context.Context, userID, noteID string) (*domain.Note, error)
	ListNotes(ctx context.Context, userID string, limit, offset int) ([]*domain.Note, int, error)
	UpdateNote(
		ctx context.Context,
		userID, noteID string,
		title, content *string,
	) (*domain.Note, error)
	DeleteNote(ctx context.Context, userID, noteID string) error
}
