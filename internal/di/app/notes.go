package app

import (
	"context"
	"fmt"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

func NewNote(userID, title, content string) *domain.Note {
	now := time.Now()
	return new(domain.Note{
		UserID:    userID,
		Title:     title,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

type NoteUseCase struct {
	noteRepo     ports.NoteRepository
	tokenService ports.TokenService
}

func NewNoteUseCase(noteRepo ports.NoteRepository, tokenService ports.TokenService) *NoteUseCase {
	return new(NoteUseCase{
		noteRepo:     noteRepo,
		tokenService: tokenService,
	})
}

func (uc *NoteUseCase) CreateNote(ctx context.Context, token, title, content string) (string, error) {
	userID, err := uc.tokenService.ValidateAccessToken(ctx, token)
	if err != nil {
		return "", domain.ErrUnauthorized
	}
	note := NewNote(userID, title, content)
	noteID, err := uc.noteRepo.Create(ctx, note)
	if err != nil {
		return "", fmt.Errorf("failed to create note: %w", err)
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
		return nil, fmt.Errorf("failed to get note: %w", err)
	}
	if note == nil {
		return nil, domain.ErrNotFound
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
		return nil, 0, fmt.Errorf("failed to list notes: %w", err)
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
		return nil, fmt.Errorf("failed to get note: %w", err)
	}
	if note == nil {
		return nil, domain.ErrNotFound
	}
	if title != "" {
		note.Title = title
	}
	if content != "" {
		note.Content = content
	}
	note.UpdatedAt = time.Now()
	if err := uc.noteRepo.Update(ctx, note); err != nil {
		return nil, fmt.Errorf("failed to update note: %w", err)
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
		return fmt.Errorf("failed to get note: %w", err)
	}
	if note == nil {
		return domain.ErrNotFound
	}
	if err := uc.noteRepo.Delete(ctx, noteID, userID); err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}
	return nil
}
