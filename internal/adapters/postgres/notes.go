package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

const (
	noteFields      = "id, user_id, title, content, created_at, updated_at"
	noteCreateQuery = "INSERT INTO notes (user_id, title, content) VALUES ($1, $2, $3)" +
		" RETURNING " + noteFields
	noteGetByIDQuery    = "SELECT " + noteFields + " FROM notes WHERE id = $1 AND user_id = $2"
	noteListByUserQuery = "SELECT " + noteFields + " FROM notes WHERE user_id = $1" +
		" ORDER BY updated_at DESC LIMIT $2 OFFSET $3"
	noteCountByUserQuery = "SELECT COUNT(*) FROM notes WHERE user_id = $1"
	noteUpdateQuery      = "UPDATE notes SET title = $1, content = $2 WHERE id = $3 AND user_id = $4"
	noteDeleteQuery      = "DELETE FROM notes WHERE id = $1 AND user_id = $2"
)

type RepositoryFactory struct {
	pool DB
}

type NoteRepository struct {
	pool DB
}

func NewRepositoryFactory(pool *pgxpool.Pool) *RepositoryFactory {
	f := new(RepositoryFactory)
	f.pool = pool

	return f
}

func NewNoteRepository(pool DB) *NoteRepository {
	r := new(NoteRepository)
	r.pool = pool

	return r
}

func (f *RepositoryFactory) NoteRepository() *NoteRepository {
	return NewNoteRepository(f.pool)
}

func (r *NoteRepository) Create(ctx context.Context, note *domain.Note) (*domain.Note, error) {
	log := logger.Method(ctx, "NoteRepository.Create")

	created, err := scanNote(
		r.pool.QueryRow(ctx, noteCreateQuery, note.UserID, note.Title, note.Content),
	)
	if err != nil {
		log.Error(ctx, "insert note failed", zap.Error(err))

		return nil, fmt.Errorf("notes.Create: %w", err)
	}

	return created, nil
}

func (r *NoteRepository) GetByID(ctx context.Context, noteID, userID string) (*domain.Note, error) {
	note, err := scanNote(r.pool.QueryRow(ctx, noteGetByIDQuery, noteID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNoteNotFound
		}

		return nil, fmt.Errorf("notes.GetByID: %w", err)
	}

	return note, nil
}

func (r *NoteRepository) ListByUserID(
	ctx context.Context,
	userID string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	var totalCount int

	countErr := r.pool.QueryRow(ctx, noteCountByUserQuery, userID).Scan(&totalCount)
	if countErr != nil {
		return nil, 0, fmt.Errorf("notes.ListByUserID count: %w", countErr)
	}

	rows, err := r.pool.Query(ctx, noteListByUserQuery, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("notes.ListByUserID: %w", err)
	}

	notes, err := scanNotes(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("notes.ListByUserID scan: %w", err)
	}

	return notes, totalCount, nil
}

func (r *NoteRepository) Update(ctx context.Context, note *domain.Note) error {
	result, err := r.pool.Exec(ctx, noteUpdateQuery, note.Title, note.Content, note.ID, note.UserID)
	if err != nil {
		return fmt.Errorf("notes.Update: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNoteNotFoundOrNotOwned
	}

	return nil
}

func (r *NoteRepository) Delete(ctx context.Context, noteID, userID string) error {
	result, err := r.pool.Exec(ctx, noteDeleteQuery, noteID, userID)
	if err != nil {
		return fmt.Errorf("notes.Delete: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNoteNotFoundOrNotOwned
	}

	return nil
}
