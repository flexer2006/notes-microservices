package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const pgUniqueViolation = "23505"

func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgUniqueViolation {
		return nil
	}

	constraint := strings.ToLower(pgErr.ConstraintName)
	switch {
	case strings.Contains(constraint, "email"), strings.Contains(pgErr.Detail, "email"):
		return domain.ErrEmailAlreadyExists
	case strings.Contains(constraint, "username"), strings.Contains(pgErr.Detail, "username"):
		return domain.ErrUserAlreadyExists
	default:
		return domain.ErrUserAlreadyExists
	}
}

func repoLogger(ctx context.Context, repository, method string) *logger.Logger {
	return logger.Method(ctx, fmt.Sprintf("%s.%s", repository, method)).
		With(zap.String("repository", repository))
}

func scanOne[T any](row pgx.Row, scanFn func(*T) []any) (*T, error) {
	out := new(T)

	scanErr := row.Scan(scanFn(out)...)
	if scanErr != nil {
		return nil, fmt.Errorf("scan row: %w", scanErr)
	}

	return out, nil
}

func scanAll[T any](rows pgx.Rows, scanFn func(*T) []any) ([]*T, error) {
	defer rows.Close()

	var result []*T

	for rows.Next() {
		item := new(T)

		scanErr := rows.Scan(scanFn(item)...)
		if scanErr != nil {
			return nil, fmt.Errorf("scan rows: %w", scanErr)
		}

		result = append(result, item)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, fmt.Errorf("rows iteration: %w", rowsErr)
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
