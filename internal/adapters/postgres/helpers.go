package postgres

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func repoLogger(ctx context.Context, repository, method string) *logger.Logger {
	return logger.Log(ctx).With(zap.String("repository", repository), zap.String("method", method))
}

func scanOne[T any](row pgx.Row, scanFn func(*T) []any) (*T, error) {
	out := new(T)
	if err := row.Scan(scanFn(out)...); err != nil {
		return nil, err
	}
	return out, nil
}

func scanAll[T any](rows pgx.Rows, scanFn func(*T) []any) ([]*T, error) {
	defer rows.Close()
	var result []*T
	for rows.Next() {
		item := new(T)
		if err := rows.Scan(scanFn(item)...); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanUser(row pgx.Row) (*domain.User, error) {
	return scanOne(row, func(u *domain.User) []any {
		return []any{&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt}
	})
}

func scanRefreshToken(row pgx.Row) (*domain.RefreshToken, error) {
	return scanOne(row, func(t *domain.RefreshToken) []any {
		return []any{&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.CreatedAt, &t.IsRevoked}
	})
}

func scanNote(row pgx.Row) (*domain.Note, error) {
	return scanOne(row, func(n *domain.Note) []any {
		return []any{&n.ID, &n.UserID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt}
	})
}

func scanNotes(rows pgx.Rows) ([]*domain.Note, error) {
	return scanAll(rows, func(n *domain.Note) []any {
		return []any{&n.ID, &n.UserID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt}
	})
}
