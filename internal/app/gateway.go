package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

const (
	profileCacheTTL = 15 * time.Minute
	cacheOpTimeout  = 5 * time.Second
)

func isInfraError(err error) bool {
	return errors.Is(err, domain.ErrServiceUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

type AuthService struct {
	authClient ports.AuthBackend
	cache      ports.Cache
	resilience *fault.ServiceResilience
}

type NotesService struct {
	notesClient ports.NotesBackend
	cache       ports.Cache
	resilience  *fault.ServiceResilience
}

func NewAuthService(authClient ports.AuthBackend, cache ports.Cache) *AuthService {
	return new(
		AuthService{
			authClient: authClient,
			cache:      cache,
			resilience: fault.NewServiceResilience("auth-service", isInfraError),
		},
	)
}

func NewNotesService(notesClient ports.NotesBackend, cache ports.Cache) *NotesService {
	return new(
		NotesService{
			notesClient: notesClient,
			cache:       cache,
			resilience:  fault.NewServiceResilience("notes-service", isInfraError),
		},
	)
}

func (s *AuthService) Register(
	ctx context.Context,
	email, username, password string,
) (*domain.TokenPair, error) {
	return fault.ExecuteCircuitOnlyResult(
		ctx,
		s.resilience,
		"Register",
		func() (*domain.TokenPair, error) {
			pair, err := s.authClient.Register(ctx, email, username, password)
			if err != nil {
				return nil, fmt.Errorf("register: %w", err)
			}

			return pair, nil
		},
	)
}

func (s *AuthService) Login(
	ctx context.Context,
	email, password string,
) (*domain.TokenPair, error) {
	return fault.ExecuteCircuitOnlyResult(
		ctx,
		s.resilience,
		"Login",
		func() (*domain.TokenPair, error) {
			pair, err := s.authClient.Login(ctx, email, password)
			if err != nil {
				return nil, fmt.Errorf("login: %w", err)
			}

			return pair, nil
		},
	)
}

func (s *AuthService) RefreshTokens(
	ctx context.Context,
	refreshToken string,
) (*domain.TokenPair, error) {
	return fault.ExecuteCircuitOnlyResult(
		ctx,
		s.resilience,
		"RefreshTokens",
		func() (*domain.TokenPair, error) {
			pair, err := s.authClient.RefreshTokens(ctx, refreshToken)
			if err != nil {
				return nil, fmt.Errorf("refresh tokens: %w", err)
			}

			return pair, nil
		},
	)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return s.resilience.ExecuteCircuitOnly(ctx, "Logout", func() error {
		err := s.authClient.Logout(ctx, refreshToken)
		if err != nil {
			return fmt.Errorf("logout: %w", err)
		}

		return nil
	})
}

func (s *AuthService) GetUserProfile(
	ctx context.Context,
	accessToken string,
) (*domain.User, error) {
	remaining := accessTokenTTLRemaining(accessToken)
	hintKey := profileTokenHintKey(hashToken(accessToken))

	if cached := s.cachedProfile(ctx, hintKey, remaining); cached != nil {
		return cached, nil
	}

	profile, err := fault.ExecuteWithResilienceResult(
		ctx,
		s.resilience,
		"GetUserProfile",
		func() (*domain.User, error) {
			user, err := s.authClient.GetUserProfile(ctx, accessToken)
			if err != nil {
				return nil, fmt.Errorf("get profile: %w", err)
			}

			return user, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if remaining > 0 && profile.ID != "" {
		profileJSON, marshalErr := json.Marshal(profile)
		if marshalErr == nil {
			cacheCtx, cancel := context.WithTimeout(ctx, cacheOpTimeout)
			defer cancel()

			ttl := min(remaining, profileCacheTTL)
			_ = s.cache.Set(cacheCtx, profileCacheKey(profile.ID), string(profileJSON), ttl)
			_ = s.cache.Set(cacheCtx, hintKey, profile.ID, ttl)
		}
	}

	return profile, nil
}

func (s *AuthService) cachedProfile(
	ctx context.Context,
	hintKey string,
	remaining time.Duration,
) *domain.User {
	if remaining <= 0 {
		return nil
	}

	userID, hintErr := s.cache.Get(ctx, hintKey)
	if hintErr != nil || userID == "" {
		return nil
	}

	cached, cacheErr := s.cache.Get(ctx, profileCacheKey(userID))
	if cacheErr != nil || cached == "" {
		return nil
	}

	profile := new(domain.User)

	unmarshalErr := json.Unmarshal([]byte(cached), profile)
	if unmarshalErr != nil {
		return nil
	}

	return profile
}

func profileCacheKey(userID string) string {
	return "profile:user:" + userID
}

func profileTokenHintKey(tokenHash string) string {
	return "profile:tok:" + tokenHash
}

func accessTokenTTLRemaining(accessToken string) time.Duration {
	claims := new(jwt.RegisteredClaims)

	_, _, parseErr := jwt.NewParser().ParseUnverified(accessToken, claims)
	if parseErr != nil || claims.ExpiresAt == nil {
		return 0
	}

	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining <= 0 {
		return 0
	}

	return remaining
}

func (s *NotesService) CreateNote(
	ctx context.Context,
	accessToken, title, content string,
) (*domain.Note, error) {
	return fault.ExecuteCircuitOnlyResult(
		ctx,
		s.resilience,
		"CreateNote",
		func() (*domain.Note, error) {
			note, err := s.notesClient.CreateNote(ctx, accessToken, title, content)
			if err != nil {
				return nil, fmt.Errorf("create note: %w", err)
			}

			return note, nil
		},
	)
}

func (s *NotesService) GetNote(
	ctx context.Context,
	accessToken, noteID string,
) (*domain.Note, error) {
	return fault.ExecuteWithResilienceResult(
		ctx,
		s.resilience,
		"GetNote",
		func() (*domain.Note, error) {
			note, err := s.notesClient.GetNote(ctx, accessToken, noteID)
			if err != nil {
				return nil, fmt.Errorf("get note: %w", err)
			}

			return note, nil
		},
	)
}

type listNotesResult struct {
	notes []*domain.Note
	total int
}

func (s *NotesService) ListNotes(
	ctx context.Context,
	accessToken string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	result, err := fault.ExecuteWithResilienceResult(
		ctx,
		s.resilience,
		"ListNotes",
		func() (listNotesResult, error) {
			notes, total, err := s.notesClient.ListNotes(
				ctx,
				accessToken,
				clampNonNegative(limit),
				clampNonNegative(offset),
			)
			if err != nil {
				return listNotesResult{}, fmt.Errorf("list notes: %w", err)
			}

			return listNotesResult{notes: notes, total: total}, nil
		},
	)
	if err != nil {
		return nil, 0, err
	}

	return result.notes, result.total, nil
}

func (s *NotesService) UpdateNote(
	ctx context.Context,
	accessToken, noteID string,
	title, content *string,
) (*domain.Note, error) {
	return fault.ExecuteCircuitOnlyResult(
		ctx,
		s.resilience,
		"UpdateNote",
		func() (*domain.Note, error) {
			note, err := s.notesClient.UpdateNote(ctx, accessToken, noteID, title, content)
			if err != nil {
				return nil, fmt.Errorf("update note: %w", err)
			}

			return note, nil
		},
	)
}

func (s *NotesService) DeleteNote(ctx context.Context, accessToken, noteID string) error {
	return s.resilience.ExecuteCircuitOnly(ctx, "DeleteNote", func() error {
		err := s.notesClient.DeleteNote(ctx, accessToken, noteID)
		if err != nil {
			return fmt.Errorf("delete note: %w", err)
		}

		return nil
	})
}

func clampNonNegative(value int) int {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}

	if value < 0 {
		return 0
	}

	return value
}
