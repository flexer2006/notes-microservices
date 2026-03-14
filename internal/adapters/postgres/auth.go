package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	msgUserNotFound            = "user not found"
	msgUserNotFoundForUpdate   = "user not found for update"
	msgUserNotFoundForDeletion = "user not found for deletion"
	msgErrorFindingUser        = "error finding user by "
	msgErrorCreatingUser       = "error creating user"
	msgErrorUpdatingUser       = "error updating user"
	msgErrorDeletingUser       = "error deleting user"
	errMsgQueryingUser         = "error querying user by "
	errMsgCreatingUser         = "error creating user"
	errMsgUpdatingUser         = "error updating user"
	errMsgDeletingUser         = "error deleting user"
)

type PgxPoolInterface interface {
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Close()
}

type UserRepository struct {
	pool PgxPoolInterface
}

func NewUserRepository(pool PgxPoolInterface) ports.UserRepository {
	return new(UserRepository{pool: pool})
}

func (r *UserRepository) findUser(ctx context.Context, query string, fieldName string, fieldValue string, logMethod string) (*domain.User, error) {
	log := logger.Log(ctx).With(zap.String("repository", "user"), zap.String("method", logMethod))
	var user domain.User
	err := r.pool.QueryRow(ctx, query, fieldValue).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, msgUserNotFound, zap.String(fieldName, fieldValue))
			return nil, domain.ErrUserNotFound
		}
		log.Error(ctx, msgErrorFindingUser+fieldName, zap.Error(err))
		return nil, fmt.Errorf(errMsgQueryingUser+"%s: %w", fieldName, err)
	}
	return new(user), nil
}

func (r *UserRepository) FindByID(ctx context.Context, idn string) (*domain.User, error) {
	query := `
        SELECT id, email, username, password_hash, created_at, updated_at
        FROM users
        WHERE id = $1
    `
	return r.findUser(ctx, query, "id", idn, "FindByID")
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
        SELECT id, email, username, password_hash, created_at, updated_at
        FROM users
        WHERE email = $1
    `
	return r.findUser(ctx, query, "email", email, "FindByEmail")
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	log := logger.Log(ctx).With(zap.String("repository", "user"), zap.String("method", "Create"))
	query := `
        INSERT INTO users (email, username, password_hash)
        VALUES ($1, $2, $3)
        RETURNING id, email, username, password_hash, created_at, updated_at
    `
	var createdUser domain.User
	err := r.pool.QueryRow(ctx, query,
		user.Email,
		user.Username,
		user.PasswordHash,
	).Scan(
		&createdUser.ID,
		&createdUser.Email,
		&createdUser.Username,
		&createdUser.PasswordHash,
		&createdUser.CreatedAt,
		&createdUser.UpdatedAt,
	)
	if err != nil {
		log.Error(ctx, msgErrorCreatingUser, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", errMsgCreatingUser, err)
	}
	return new(createdUser), nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	log := logger.Log(ctx).With(zap.String("repository", "user"), zap.String("method", "Update"))
	query := `
        UPDATE users
        SET email = $2, username = $3, password_hash = $4, updated_at = $5
        WHERE id = $1
        RETURNING id, email, username, password_hash, created_at, updated_at
    `
	var updatedUser domain.User
	now := time.Now().UTC()
	err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.Username,
		user.PasswordHash,
		now,
	).Scan(
		&updatedUser.ID,
		&updatedUser.Email,
		&updatedUser.Username,
		&updatedUser.PasswordHash,
		&updatedUser.CreatedAt,
		&updatedUser.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, msgUserNotFoundForUpdate, zap.String("id", user.ID))
			return nil, domain.ErrUserNotFound
		}
		log.Error(ctx, msgErrorUpdatingUser, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", errMsgUpdatingUser, err)
	}
	return new(updatedUser), nil
}

func (r *UserRepository) Delete(ctx context.Context, idn string) error {
	log := logger.Log(ctx).With(zap.String("repository", "user"), zap.String("method", "Delete"))
	query := `
        DELETE FROM users
        WHERE id = $1
    `
	result, err := r.pool.Exec(ctx, query, idn)
	if err != nil {
		log.Error(ctx, msgErrorDeletingUser, zap.Error(err))
		return fmt.Errorf("%s: %w", errMsgDeletingUser, err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, msgUserNotFoundForDeletion, zap.String("id", idn))
		return domain.ErrUserNotFound
	}
	return nil
}

type AuthRepositoryFactory struct {
	userRepo  ports.UserRepository
	tokenRepo ports.TokenRepository
}

func NewAuthRepositoryFactory(pool *pgxpool.Pool) *AuthRepositoryFactory {
	return new(AuthRepositoryFactory{
		userRepo:  NewUserRepository(pool),
		tokenRepo: NewTokenRepository(pool),
	})
}

func (f *AuthRepositoryFactory) UserRepository() ports.UserRepository {
	return f.userRepo
}

func (f *AuthRepositoryFactory) TokenRepository() ports.TokenRepository {
	return f.tokenRepo
}

type TokenRepository struct {
	pool PgxPoolInterface
}

func NewTokenRepository(pool PgxPoolInterface) ports.TokenRepository {
	return new(TokenRepository{pool: pool})
}

func (r *TokenRepository) FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	log := logger.Log(ctx).With(zap.String("repository", "token"), zap.String("method", "FindByToken"))
	query := `
        SELECT id, user_id, token, expires_at, created_at, is_revoked
        FROM refresh_tokens
        WHERE token = $1
    `
	var refreshToken domain.RefreshToken
	var idn string
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&idn,
		&refreshToken.UserID,
		&refreshToken.Token,
		&refreshToken.ExpiresAt,
		&refreshToken.CreatedAt,
		&refreshToken.IsRevoked,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, "token not found")
			return nil, domain.ErrInvalidRefreshToken
		}
		log.Error(ctx, "error finding refresh token", zap.Error(err))
		return nil, fmt.Errorf("error querying refresh token: %w", err)
	}
	refreshToken.ID = idn
	return new(refreshToken), nil
}

func (r *TokenRepository) StoreRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	log := logger.Log(ctx).With(zap.String("repository", "token"), zap.String("method", "StoreRefreshToken"))
	query := `
        INSERT INTO refresh_tokens (user_id, token, expires_at, is_revoked)
        VALUES ($1, $2, $3, $4)
    `
	_, err := r.pool.Exec(ctx, query,
		token.UserID,
		token.Token,
		token.ExpiresAt,
		token.IsRevoked,
	)
	if err != nil {
		log.Error(ctx, "error storing refresh token", zap.Error(err))
		return fmt.Errorf("error storing refresh token: %w", err)
	}
	return nil
}

func (r *TokenRepository) RevokeToken(ctx context.Context, token string) error {
	log := logger.Log(ctx).With(zap.String("repository", "token"), zap.String("method", "RevokeToken"))
	query := `
        UPDATE refresh_tokens
        SET is_revoked = true
        WHERE token = $1
    `
	result, err := r.pool.Exec(ctx, query, token)
	if err != nil {
		log.Error(ctx, "error revoking refresh token", zap.Error(err))
		return fmt.Errorf("error revoking refresh token: %w", err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, "token not found for revocation")
		return domain.ErrInvalidRefreshToken
	}
	return nil
}

func (r *TokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	log := logger.Log(ctx).With(zap.String("repository", "token"), zap.String("method", "CleanupExpiredTokens"))
	query := `
        DELETE FROM refresh_tokens
        WHERE expires_at < NOW() OR is_revoked = true
    `
	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		log.Error(ctx, "error cleaning up expired tokens", zap.Error(err))
		return fmt.Errorf("error cleaning up expired tokens: %w", err)
	}
	log.Info(ctx, "expired tokens cleaned up", zap.Int64("removed_count", result.RowsAffected()))
	return nil
}

func (r *TokenRepository) FindUserTokens(ctx context.Context, userID string) ([]*domain.RefreshToken, error) {
	log := logger.Log(ctx).With(
		zap.String("repository", "token"),
		zap.String("method", "FindUserTokens"),
		zap.String("userID", userID),
	)
	query := `
        SELECT id, user_id, token, expires_at, created_at, is_revoked
        FROM refresh_tokens
        WHERE user_id = $1
        ORDER BY created_at DESC
    `
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		log.Error(ctx, "error querying user tokens", zap.Error(err))
		return nil, fmt.Errorf("error querying user tokens: %w", err)
	}
	defer rows.Close()
	var tokens []*domain.RefreshToken
	for rows.Next() {
		var token domain.RefreshToken
		var idn string
		err := rows.Scan(
			&idn,
			&token.UserID,
			&token.Token,
			&token.ExpiresAt,
			&token.CreatedAt,
			&token.IsRevoked,
		)
		if err != nil {
			log.Error(ctx, "error scanning token row", zap.Error(err))
			return nil, fmt.Errorf("error scanning token row: %w", err)
		}
		token.ID = idn
		tokens = append(tokens, &token)
	}
	if err = rows.Err(); err != nil {
		log.Error(ctx, "error iterating token rows", zap.Error(err))
		return nil, fmt.Errorf("error iterating token rows: %w", err)
	}
	return tokens, nil
}

func (r *TokenRepository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	log := logger.Log(ctx).With(
		zap.String("repository", "token"),
		zap.String("method", "RevokeAllUserTokens"),
		zap.String("userID", userID),
	)
	query := `
        UPDATE refresh_tokens
        SET is_revoked = true
        WHERE user_id = $1 AND is_revoked = false
    `
	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		log.Error(ctx, "error revoking all user tokens", zap.Error(err))
		return fmt.Errorf("error revoking all user tokens: %w", err)
	}
	log.Info(ctx, "all user tokens revoked", zap.Int64("count", result.RowsAffected()))
	return nil
}
