package app

import (
	"context"
	"fmt"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

const (
	defaultListLimit = 10
	maxListLimit     = 100
)

type NoteUseCase struct {
	noteRepo ports.NoteRepository
}

func NewNoteUseCase(noteRepo ports.NoteRepository) *NoteUseCase {
	return new(NoteUseCase{noteRepo: noteRepo})
}

func (uc *NoteUseCase) CreateNote(
	ctx context.Context,
	userID, title, content string,
) (*domain.Note, error) {
	note, err := domain.NewNote(userID, title, content)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	created, err := uc.noteRepo.Create(ctx, note)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	return created, nil
}

func (uc *NoteUseCase) GetNote(ctx context.Context, userID, noteID string) (*domain.Note, error) {
	note, err := uc.noteRepo.GetByID(ctx, noteID, userID)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}

	return note, nil
}

func (uc *NoteUseCase) ListNotes(
	ctx context.Context,
	userID string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	if limit > maxListLimit {
		limit = maxListLimit
	}

	if offset < 0 {
		offset = 0
	}

	notes, total, err := uc.noteRepo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list notes: %w", err)
	}

	return notes, total, nil
}

func (uc *NoteUseCase) UpdateNote(
	ctx context.Context,
	userID, noteID string,
	title, content *string,
) (*domain.Note, error) {
	note, err := uc.noteRepo.GetByID(ctx, noteID, userID)
	if err != nil {
		return nil, fmt.Errorf("update note: %w", err)
	}

	applyErr := note.ApplyUpdate(title, content)
	if applyErr != nil {
		return nil, fmt.Errorf("update note: %w", applyErr)
	}

	updateErr := uc.noteRepo.Update(ctx, note)
	if updateErr != nil {
		return nil, fmt.Errorf("update note: %w", updateErr)
	}

	return note, nil
}

func (uc *NoteUseCase) DeleteNote(ctx context.Context, userID, noteID string) error {
	err := uc.noteRepo.Delete(ctx, noteID, userID)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}

	return nil
}
