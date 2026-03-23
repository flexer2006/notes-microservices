package app

import (
	"context"
	"fmt"
	"time"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

type NotesServiceClient interface {
	CreateNote(ctx context.Context, title, content string) (*notesv1.NoteResponse, error)
	UpdateNote(ctx context.Context, noteID string, title, content *string) (*notesv1.NoteResponse, error)
	ListNotes(ctx context.Context, limit, offset int32) (*notesv1.ListNotesResponse, error)
	GetNote(ctx context.Context, noteID string) (*notesv1.NoteResponse, error)
	DeleteNote(ctx context.Context, noteID string) error
}

type NotesService struct {
	notesClient NotesServiceClient
	cache       ports.Cache
	resilience  *fault.ServiceResilience
}

type NoteUseCase struct {
	noteRepo     ports.NoteRepository
	tokenService ports.TokenService
}

func NewNotesService(notesClient NotesServiceClient, cache ports.Cache) ports.NotesService {
	return new(NotesService{notesClient: notesClient, cache: cache, resilience: fault.NewServiceResilience("notes-service")})
}

func NewNoteUseCase(noteRepo ports.NoteRepository, tokenService ports.TokenService) *NoteUseCase {
	return new(NoteUseCase{noteRepo: noteRepo, tokenService: tokenService})
}

func (uc *NoteUseCase) CreateNote(ctx context.Context, token, title, content string) (string, error) {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return "", domain.ErrUnauthorized
	}
	note := new(domain.Note{UserID: userID, Title: title, Content: content, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	noteID, err := uc.noteRepo.Create(ctx, note)
	if err != nil {
		return "", fmt.Errorf("%s: %w", domain.ErrFailedToCreateNote, err)
	}
	return noteID, nil
}

func (uc *NoteUseCase) GetNote(ctx context.Context, token, noteID string) (*domain.Note, error) {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	note, err := uc.noteRepo.GetByID(ctx, noteID, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToGetNote, err)
	}
	if note == nil {
		return nil, domain.ErrNoteNotFound
	}
	return note, nil
}

func (uc *NoteUseCase) ListNotes(ctx context.Context, token string, limit, offset int) ([]*domain.Note, int, error) {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, 0, domain.ErrUnauthorized
	}
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	notes, total, err := uc.noteRepo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", domain.ErrFailedToListNotes, err)
	}
	return notes, total, nil
}

func (uc *NoteUseCase) UpdateNote(ctx context.Context, token, noteID, title, content string) (*domain.Note, error) {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	note, err := uc.noteRepo.GetByID(ctx, noteID, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToGetNote, err)
	}
	if note == nil {
		return nil, domain.ErrNoteNotFound
	}
	if title != "" {
		note.Title = title
	}
	if content != "" {
		note.Content = content
	}
	note.UpdatedAt = time.Now()
	if err := uc.noteRepo.Update(ctx, note); err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToUpdateNote, err)
	}
	return note, nil
}

func (uc *NoteUseCase) DeleteNote(ctx context.Context, token, noteID string) error {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return domain.ErrUnauthorized
	}
	note, err := uc.noteRepo.GetByID(ctx, noteID, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", domain.ErrFailedToGetNote, err)
	}
	if note == nil {
		return domain.ErrNoteNotFound
	}
	if err := uc.noteRepo.Delete(ctx, noteID, userID); err != nil {
		return fmt.Errorf("%s: %w", domain.ErrFailedToDeleteNote, err)
	}
	return nil
}
