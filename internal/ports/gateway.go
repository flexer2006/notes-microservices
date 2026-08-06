package ports

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

type AuthService interface {
	Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error)
	Login(ctx context.Context, email, password string) (*domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	GetUserProfile(ctx context.Context, accessToken string) (*domain.User, error)
}

//nolint:iface // intentional: accessToken vs userID roles; not the same port.
type NotesService interface {
	CreateNote(ctx context.Context, accessToken, title, content string) (*domain.Note, error)
	UpdateNote(
		ctx context.Context,
		accessToken, noteID string,
		title, content *string,
	) (*domain.Note, error)
	ListNotes(
		ctx context.Context,
		accessToken string,
		limit, offset int,
	) ([]*domain.Note, int, error)
	GetNote(ctx context.Context, accessToken, noteID string) (*domain.Note, error)
	DeleteNote(ctx context.Context, accessToken, noteID string) error
}
