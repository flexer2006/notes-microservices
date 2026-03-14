package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type RepositoryFactory struct {
	pool DBPool
}

func NewRepositoryFactory(pool *pgxpool.Pool) *RepositoryFactory {
	return &RepositoryFactory{pool: pool}
}

func (f *RepositoryFactory) NoteRepository() ports.NoteRepository {
	return NewNoteRepository(f.pool)
}

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type NoteRepository struct {
	pool DBPool
}

var ErrNoteNotFoundOrNotOwned = errors.New("note not found or not owned by user")

func NewNoteRepository(pool DBPool) ports.NoteRepository {
	return new(NoteRepository{pool: pool})
}

func (r *NoteRepository) Create(ctx context.Context, note *domain.Note) (string, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Create"))
	log.Debug(ctx, "creating new note", zap.String("userID", note.UserID))
	var noteID string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notes (user_id, title, content) VALUES ($1, $2, $3) RETURNING id`,
		note.UserID, note.Title, note.Content,
	).Scan(&noteID)
	if err != nil {
		log.Error(ctx, "failed to create note", zap.Error(err))
		return "", fmt.Errorf("failed to create note: %w", err)
	}
	log.Debug(ctx, "note created", zap.String("noteID", noteID))
	return noteID, nil
}

func (r *NoteRepository) GetByID(ctx context.Context, noteID, userID string) (*domain.Note, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.GetByID"))
	log.Debug(ctx, "getting note", zap.String("noteID", noteID), zap.String("userID", userID))
	var note domain.Note
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, title, content, created_at, updated_at 
         FROM notes 
         WHERE id = $1 AND user_id = $2`,
		noteID, userID,
	).Scan(&note.ID, &note.UserID, &note.Title, &note.Content, &note.CreatedAt, &note.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, "note not found", zap.String("noteID", noteID))
			return nil, nil
		}
		log.Error(ctx, "failed to get note", zap.Error(err))
		return nil, fmt.Errorf("failed to get note: %w", err)
	}
	return new(note), nil
}

func (r *NoteRepository) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Note, int, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.ListByUserID"))
	log.Debug(ctx, "listing notes", zap.String("userID", userID), zap.Int("limit", limit), zap.Int("offset", offset))
	var totalCount int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notes WHERE user_id = $1`,
		userID,
	).Scan(&totalCount)
	if err != nil {
		log.Error(ctx, "failed to count notes", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to count notes: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, title, content, created_at, updated_at 
         FROM notes 
         WHERE user_id = $1 
         ORDER BY updated_at DESC 
         LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		log.Error(ctx, "failed to list notes", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list notes: %w", err)
	}
	defer rows.Close()
	notes := make([]*domain.Note, 0)
	for rows.Next() {
		var note domain.Note
		err := rows.Scan(&note.ID, &note.UserID, &note.Title, &note.Content, &note.CreatedAt, &note.UpdatedAt)
		if err != nil {
			log.Error(ctx, "failed to scan note", zap.Error(err))
			return nil, 0, fmt.Errorf("failed to scan note: %w", err)
		}
		notes = append(notes, &note)
	}
	if err := rows.Err(); err != nil {
		log.Error(ctx, "error iterating rows", zap.Error(err))
		return nil, 0, fmt.Errorf("error iterating rows: %w", err)
	}
	return notes, totalCount, nil
}

func (r *NoteRepository) Update(ctx context.Context, note *domain.Note) error {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Update"))
	log.Debug(ctx, "updating note", zap.String("noteID", note.ID))
	result, err := r.pool.Exec(ctx,
		`UPDATE notes SET title = $1, content = $2 WHERE id = $3 AND user_id = $4`,
		note.Title, note.Content, note.ID, note.UserID,
	)
	if err != nil {
		log.Error(ctx, "failed to update note", zap.Error(err))
		return fmt.Errorf("failed to update note: %w", err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, "note not found or not owned by user")
		return ErrNoteNotFoundOrNotOwned
	}
	return nil
}

func (r *NoteRepository) Delete(ctx context.Context, noteID, userID string) error {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Delete"))
	log.Debug(ctx, "deleting note", zap.String("noteID", noteID))
	result, err := r.pool.Exec(ctx,
		`DELETE FROM notes WHERE id = $1 AND user_id = $2`,
		noteID, userID,
	)
	if err != nil {
		log.Error(ctx, "failed to delete note", zap.Error(err))
		return fmt.Errorf("failed to delete note: %w", err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, "note not found or not owned by user")
		return ErrNoteNotFoundOrNotOwned
	}
	return nil
}
