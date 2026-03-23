package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	userFields                = "id, email, username, password_hash, created_at, updated_at"
	userFindByIDQuery         = "SELECT " + userFields + " FROM users WHERE id = $1"
	userFindByEmailQuery      = "SELECT " + userFields + " FROM users WHERE email = $1"
	userCreateQuery           = "INSERT INTO users (email, username, password_hash) VALUES ($1, $2, $3) RETURNING " + userFields
	userUpdateQuery           = "UPDATE users SET email = $2, username = $3, password_hash = $4, updated_at = $5 WHERE id = $1 RETURNING " + userFields
	userDeleteQuery           = "DELETE FROM users WHERE id = $1"
	tokenFields               = "id, user_id, token, expires_at, created_at, is_revoked"
	tokenFindByTokenQuery     = "SELECT " + tokenFields + " FROM refresh_tokens WHERE token = $1"
	tokenInsertQuery          = "INSERT INTO refresh_tokens (user_id, token, expires_at, is_revoked) VALUES ($1, $2, $3, $4)"
	tokenRevokeQuery          = "UPDATE refresh_tokens SET is_revoked = true WHERE token = $1"
	tokenCleanupQuery         = "DELETE FROM refresh_tokens WHERE expires_at < NOW() OR is_revoked = true"
	tokenFindByUserQuery      = "SELECT " + tokenFields + " FROM refresh_tokens WHERE user_id = $1 ORDER BY created_at DESC"
	tokenRevokeAllByUserQuery = "UPDATE refresh_tokens SET is_revoked = true WHERE user_id = $1 AND is_revoked = false"
)

type UserRepository struct {
	pool DB
}

func NewUserRepository(pool DB) ports.UserRepository {
	r := new(UserRepository)
	r.pool = pool
	return r
}

type TokenRepository struct {
	pool DB
}

func NewTokenRepository(pool DB) ports.TokenRepository {
	r := new(TokenRepository)
	r.pool = pool
	return r
}

type AuthRepositoryFactory struct {
	userRepo  ports.UserRepository
	tokenRepo ports.TokenRepository
}

func NewAuthRepositoryFactory(pool *pgxpool.Pool) *AuthRepositoryFactory {
	f := new(AuthRepositoryFactory)
	f.userRepo = NewUserRepository(pool)
	f.tokenRepo = NewTokenRepository(pool)
	return f
}

func (r *UserRepository) findUser(ctx context.Context, query, fieldName, fieldValue, logMethod string) (*domain.User, error) {
	log := repoLogger(ctx, "user", logMethod)
	row := r.pool.QueryRow(ctx, query, fieldValue)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, "user not found", zap.String(fieldName, fieldValue))
			return nil, domain.ErrUserNotFound
		}
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, idn string) (*domain.User, error) {
	return r.findUser(ctx, userFindByIDQuery, "id", idn, "FindByID")
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findUser(ctx, userFindByEmailQuery, "email", email, "FindByEmail")
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	log := repoLogger(ctx, "user", "Create")
	row := r.pool.QueryRow(ctx, userCreateQuery, user.Email, user.Username, user.PasswordHash)
	createdUser, err := scanUser(row)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return createdUser, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	log := repoLogger(ctx, "user", "Update")
	now := time.Now().UTC()
	row := r.pool.QueryRow(ctx, userUpdateQuery,
		user.ID,
		user.Email,
		user.Username,
		user.PasswordHash,
		now,
	)
	updatedUser, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, "user not found for update", zap.String("id", user.ID))
			return nil, domain.ErrUserNotFound
		}
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return updatedUser, nil
}

func (r *UserRepository) Delete(ctx context.Context, idn string) error {
	log := repoLogger(ctx, "user", "Delete")
	result, err := r.pool.Exec(ctx, userDeleteQuery, idn)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, "user not found for deletion", zap.String("id", idn))
		return domain.ErrUserNotFound
	}
	return nil
}

func (f *AuthRepositoryFactory) UserRepository() ports.UserRepository {
	return f.userRepo
}

func (f *AuthRepositoryFactory) TokenRepository() ports.TokenRepository {
	return f.tokenRepo
}

func (r *TokenRepository) FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	log := repoLogger(ctx, "token", "FindByToken")
	row := r.pool.QueryRow(ctx, tokenFindByTokenQuery, token)
	refreshToken, err := scanRefreshToken(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Debug(ctx, "token not found")
			return nil, domain.ErrInvalidRefreshToken
		}
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return refreshToken, nil
}

func (r *TokenRepository) StoreRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	log := repoLogger(ctx, "token", "StoreRefreshToken")
	_, err := r.pool.Exec(ctx, tokenInsertQuery,
		token.UserID,
		token.Token,
		token.ExpiresAt,
		token.IsRevoked,
	)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return nil
}

func (r *TokenRepository) RevokeToken(ctx context.Context, token string) error {
	log := repoLogger(ctx, "token", "RevokeToken")
	result, err := r.pool.Exec(ctx, tokenRevokeQuery, token)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	if result.RowsAffected() == 0 {
		log.Debug(ctx, "token not found for revocation")
		return domain.ErrInvalidRefreshToken
	}
	return nil
}

func (r *TokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	log := repoLogger(ctx, "token", "CleanupExpiredTokens")
	result, err := r.pool.Exec(ctx, tokenCleanupQuery)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	log.Info(ctx, "expired tokens cleaned up", zap.Int64("removed_count", result.RowsAffected()))
	return nil
}

func (r *TokenRepository) FindUserTokens(ctx context.Context, userID string) ([]*domain.RefreshToken, error) {
	log := repoLogger(ctx, "token", "FindUserTokens").With(zap.String("userID", userID))
	rows, err := r.pool.Query(ctx, tokenFindByUserQuery, userID)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	tokens, err := scanAll(rows, func(t *domain.RefreshToken) []any {
		return []any{&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.CreatedAt, &t.IsRevoked}
	})
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrInvalidRequest.Error(), err)
	}
	return tokens, nil
}

func (r *TokenRepository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	log := repoLogger(ctx, "token", "RevokeAllUserTokens").With(zap.String("userID", userID))
	result, err := r.pool.Exec(ctx, tokenRevokeAllByUserQuery, userID)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrRevokingAllUserTokens, err)
	}
	log.Info(ctx, "all user tokens revoked", zap.Int64("count", result.RowsAffected()))
	return nil
}
