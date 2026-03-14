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
	r := new(RepositoryFactory)
	r.pool = pool
	return r
}

func (f *RepositoryFactory) NoteRepository() ports.NoteRepository {
	return NewNoteRepository(f.pool)
}

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const (
	noteFields           = "id, user_id, title, content, created_at, updated_at"
	noteCreateQuery      = "INSERT INTO notes (user_id, title, content) VALUES ($1, $2, $3) RETURNING id"
	noteGetByIDQuery     = "SELECT " + noteFields + " FROM notes WHERE id = $1 AND user_id = $2"
	noteListByUserQuery  = "SELECT " + noteFields + " FROM notes WHERE user_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3"
	noteCountByUserQuery = "SELECT COUNT(*) FROM notes WHERE user_id = $1"
	noteUpdateQuery      = "UPDATE notes SET title = $1, content = $2 WHERE id = $3 AND user_id = $4"
	noteDeleteQuery      = "DELETE FROM notes WHERE id = $1 AND user_id = $2"
)

type NoteRepository struct {
	pool DBPool
}

func NewNoteRepository(pool DBPool) ports.NoteRepository {
	r := new(NoteRepository)
	r.pool = pool
	return r
}

func (r *NoteRepository) Create(ctx context.Context, note *domain.Note) (string, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Create"))
	log.Debug(ctx, domain.LogRepoCreatingNote, zap.String("userID", note.UserID))
	var noteID string
	err := r.pool.QueryRow(ctx, noteCreateQuery, note.UserID, note.Title, note.Content).Scan(&noteID)
	if err != nil {
		log.Error(ctx, domain.LogErrorCreateNoteFailed, zap.Error(err))
		return "", fmt.Errorf("%s: %w", domain.ErrFailedToCreateNote, err)
	}
	log.Debug(ctx, domain.LogRepoNoteCreated, zap.String("noteID", noteID))
	return noteID, nil
}

func (r *NoteRepository) GetByID(ctx context.Context, noteID, userID string) (*domain.Note, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.GetByID"))
	log.Debug(ctx, domain.LogRepoGettingNote, zap.String("noteID", noteID), zap.String("userID", userID))
	note, err := scanNote(r.pool.QueryRow(ctx, noteGetByIDQuery, noteID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, domain.LogRepoNoteNotFound, zap.String("noteID", noteID))
			return nil, nil
		}
		log.Error(ctx, domain.LogErrorGetNoteFailed, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToGetNote, err)
	}
	return note, nil
}

func (r *NoteRepository) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Note, int, error) {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.ListByUserID"))
	log.Debug(ctx, domain.LogRepoListingNotes, zap.String("userID", userID), zap.Int("limit", limit), zap.Int("offset", offset))
	var totalCount int
	if err := r.pool.QueryRow(ctx, noteCountByUserQuery, userID).Scan(&totalCount); err != nil {
		log.Error(ctx, domain.LogErrorCountNotesFailed, zap.Error(err))
		return nil, 0, fmt.Errorf("%s: %w", domain.ErrFailedToCountNotes, err)
	}
	rows, err := r.pool.Query(ctx, noteListByUserQuery, userID, limit, offset)
	if err != nil {
		log.Error(ctx, domain.LogErrorListNotesFailed, zap.Error(err))
		return nil, 0, fmt.Errorf("%s: %w", domain.ErrFailedToListNotes, err)
	}
	notes, err := scanNotes(rows)
	if err != nil {
		log.Error(ctx, domain.LogErrorScanNoteFailed, zap.Error(err))
		return nil, 0, fmt.Errorf("%s: %w", domain.ErrFailedToScanNote, err)
	}
	return notes, totalCount, nil
}

func (r *NoteRepository) Update(ctx context.Context, note *domain.Note) error {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Update"))
	log.Debug(ctx, domain.LogRepoUpdatingNote, zap.String("noteID", note.ID))
	result, err := r.pool.Exec(ctx, noteUpdateQuery, note.Title, note.Content, note.ID, note.UserID)
	if err != nil {
		log.Error(ctx, domain.LogErrorUpdateNoteFailed, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrFailedToUpdateNote, err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, domain.ErrNoteNotFoundOrNotOwned.Error())
		return domain.ErrNoteNotFoundOrNotOwned
	}
	return nil
}

func (r *NoteRepository) Delete(ctx context.Context, noteID, userID string) error {
	log := logger.Log(ctx).With(zap.String("method", "NoteRepository.Delete"))
	log.Debug(ctx, domain.LogRepoDeletingNote, zap.String("noteID", noteID))
	result, err := r.pool.Exec(ctx, noteDeleteQuery, noteID, userID)
	if err != nil {
		log.Error(ctx, domain.LogErrorDeleteNoteFailed, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrFailedToDeleteNote, err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, domain.ErrNoteNotFoundOrNotOwned.Error())
		return domain.ErrNoteNotFoundOrNotOwned
	}
	return nil
}
