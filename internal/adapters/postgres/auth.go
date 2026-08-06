package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
)

const (
	userFields           = "id, email, username, password_hash, created_at, updated_at"
	userFindByIDQuery    = "SELECT " + userFields + " FROM users WHERE id = $1"
	userFindByEmailQuery = "SELECT " + userFields + " FROM users WHERE email = $1"
	userCreateQuery      = "INSERT INTO users (email, username, password_hash) VALUES ($1, $2, $3)" +
		" RETURNING " + userFields
	userUpdateQuery = "UPDATE users SET email = $2, username = $3, password_hash = $4," +
		" updated_at = $5 WHERE id = $1 RETURNING " + userFields
	userDeleteQuery = "DELETE FROM users WHERE id = $1"

	// #nosec G101
	tokenFields           = "id, user_id, token, expires_at, created_at, is_revoked"
	tokenFindByTokenQuery = "SELECT " + tokenFields + " FROM refresh_tokens WHERE token = $1"
	tokenInsertQuery      = "INSERT INTO refresh_tokens (user_id, token, expires_at, is_revoked)" +
		" VALUES ($1, $2, $3, $4)"
	tokenRevokeQuery        = "UPDATE refresh_tokens SET is_revoked = true WHERE token = $1"
	tokenConsumeActiveQuery = "UPDATE refresh_tokens SET is_revoked = true" +
		" WHERE token = $1 AND is_revoked = false AND expires_at > NOW()" +
		" RETURNING " + tokenFields
	// #nosec G101
	tokenCleanupQuery    = "DELETE FROM refresh_tokens WHERE expires_at < NOW() OR is_revoked = true"
	tokenFindByUserQuery = "SELECT " + tokenFields + " FROM refresh_tokens WHERE user_id = $1" +
		" ORDER BY created_at DESC"
	tokenRevokeAllByUserQuery = "UPDATE refresh_tokens SET is_revoked = true" +
		" WHERE user_id = $1 AND is_revoked = false"
)

type UserRepository struct {
	pool DB
}

func NewUserRepository(pool DB) *UserRepository {
	r := new(UserRepository)
	r.pool = pool

	return r
}

type TokenRepository struct {
	pool *pgxpool.Pool
}

func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	r := new(TokenRepository)
	r.pool = pool

	return r
}

type AuthRepositoryFactory struct {
	userRepo  *UserRepository
	tokenRepo *TokenRepository
}

func NewAuthRepositoryFactory(pool *pgxpool.Pool) *AuthRepositoryFactory {
	f := new(AuthRepositoryFactory)
	f.userRepo = NewUserRepository(pool)
	f.tokenRepo = NewTokenRepository(pool)

	return f
}

func (f *AuthRepositoryFactory) UserRepository() *UserRepository {
	return f.userRepo
}

func (f *AuthRepositoryFactory) TokenRepository() *TokenRepository {
	return f.tokenRepo
}

func (r *UserRepository) FindByID(ctx context.Context, idn string) (*domain.User, error) {
	return r.findUser(ctx, userFindByIDQuery, "FindByID", idn)
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findUser(ctx, userFindByEmailQuery, "FindByEmail", email)
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	createdUser, err := scanUser(
		r.pool.QueryRow(ctx, userCreateQuery, user.Email, user.Username, user.PasswordHash),
	)
	if err != nil {
		mapped := mapUniqueViolation(err)
		if mapped != nil {
			return nil, mapped
		}

		return nil, fmt.Errorf("users.Create: %w", err)
	}

	return createdUser, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, userUpdateQuery,
		user.ID,
		user.Email,
		user.Username,
		user.PasswordHash,
		time.Now().UTC(),
	)

	updatedUser, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}

		mapped := mapUniqueViolation(err)
		if mapped != nil {
			return nil, mapped
		}

		return nil, fmt.Errorf("users.Update: %w", err)
	}

	return updatedUser, nil
}

func (r *UserRepository) Delete(ctx context.Context, idn string) error {
	result, err := r.pool.Exec(ctx, userDeleteQuery, idn)
	if err != nil {
		return fmt.Errorf("users.Delete: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

func (r *UserRepository) findUser(
	ctx context.Context,
	query, logMethod, fieldValue string,
) (*domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, query, fieldValue))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}

		return nil, fmt.Errorf("users.%s: %w", logMethod, err)
	}

	return user, nil
}

func (r *TokenRepository) FindByToken(
	ctx context.Context,
	tokenHash string,
) (*domain.RefreshToken, error) {
	refreshToken, err := scanRefreshToken(r.pool.QueryRow(ctx, tokenFindByTokenQuery, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInvalidRefreshToken
		}

		return nil, fmt.Errorf("tokens.FindByToken: %w", err)
	}

	return refreshToken, nil
}

func (r *TokenRepository) StoreRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	_, err := r.pool.Exec(ctx, tokenInsertQuery,
		token.UserID,
		token.Token,
		token.ExpiresAt,
		token.IsRevoked,
	)
	if err != nil {
		return fmt.Errorf("tokens.StoreRefreshToken: %w", err)
	}

	return nil
}

func (r *TokenRepository) RevokeToken(ctx context.Context, tokenHash string) error {
	result, err := r.pool.Exec(ctx, tokenRevokeQuery, tokenHash)
	if err != nil {
		return fmt.Errorf("tokens.RevokeToken: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrInvalidRefreshToken
	}

	return nil
}

func (r *TokenRepository) ConsumeActiveToken(
	ctx context.Context,
	tokenHash string,
) (*domain.RefreshToken, error) {
	refreshToken, err := scanRefreshToken(r.pool.QueryRow(ctx, tokenConsumeActiveQuery, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInvalidRefreshToken
		}

		return nil, fmt.Errorf("tokens.ConsumeActiveToken: %w", err)
	}

	return refreshToken, nil
}

func (r *TokenRepository) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	newToken *domain.RefreshToken,
) (*domain.RefreshToken, error) {
	if newToken == nil {
		return nil, fmt.Errorf("tokens.RotateRefreshToken: %w", domain.ErrInvalidParams)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("tokens.RotateRefreshToken begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	consumed, err := scanRefreshToken(tx.QueryRow(ctx, tokenConsumeActiveQuery, oldHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInvalidRefreshToken
		}

		return nil, fmt.Errorf("tokens.RotateRefreshToken consume: %w", err)
	}

	if newToken.UserID != "" && consumed.UserID != newToken.UserID {
		return nil, domain.ErrInvalidRefreshToken
	}

	_, err = tx.Exec(
		ctx,
		tokenInsertQuery,
		consumed.UserID,
		newToken.Token,
		newToken.ExpiresAt,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("tokens.RotateRefreshToken insert: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("tokens.RotateRefreshToken commit: %w", err)
	}

	return consumed, nil
}

func (r *TokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	result, err := r.pool.Exec(ctx, tokenCleanupQuery)
	if err != nil {
		return fmt.Errorf("tokens.CleanupExpiredTokens: %w", err)
	}

	repoLogger(
		ctx,
		"token",
		"CleanupExpiredTokens",
	).Info(ctx, "expired tokens cleaned up", zap.Int64("removed_count", result.RowsAffected()))

	return nil
}

func (r *TokenRepository) FindUserTokens(
	ctx context.Context,
	userID string,
) ([]*domain.RefreshToken, error) {
	rows, err := r.pool.Query(ctx, tokenFindByUserQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("tokens.FindUserTokens: %w", err)
	}

	tokens, err := scanAll(rows, func(t *domain.RefreshToken) []any {
		return []any{&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.CreatedAt, &t.IsRevoked}
	})
	if err != nil {
		return nil, fmt.Errorf("tokens.FindUserTokens scan: %w", err)
	}

	return tokens, nil
}

func (r *TokenRepository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	result, err := r.pool.Exec(ctx, tokenRevokeAllByUserQuery, userID)
	if err != nil {
		return fmt.Errorf("tokens.RevokeAllUserTokens: %w", err)
	}

	repoLogger(
		ctx,
		"token",
		"RevokeAllUserTokens",
	).Info(ctx, "all user tokens revoked", zap.Int64("count", result.RowsAffected()))

	return nil
}
