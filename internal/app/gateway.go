package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

func (s *AuthService) Register(ctx context.Context, email, username, password string) (*domain.TokenPair, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceRegister)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "Register", func() (any, error) {
		response, err := s.authClient.Register(ctx, email, username, password)
		if err != nil {
			log.Error(ctx, domain.LogErrorRegisterFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrUserRegistrationFailed, err)
		}
		return &domain.TokenPair{UserID: response.UserId, AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, ExpiresAt: response.ExpiresAt.AsTime()}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrUserRegistrationFailed, err)
	}
	return result.(*domain.TokenPair), nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*domain.TokenPair, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceLogin)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "Login", func() (any, error) {
		response, err := s.authClient.Login(ctx, email, password)
		if err != nil {
			log.Error(ctx, domain.LogErrorLoginFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrLogin, err)
		}
		return &domain.TokenPair{UserID: response.UserId, Username: response.Username, AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, ExpiresAt: response.ExpiresAt.AsTime()}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrLogin, err)
	}
	return result.(*domain.TokenPair), nil
}

func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceTokenRefresh)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "RefreshTokens", func() (any, error) {
		response, err := s.authClient.RefreshTokens(ctx, refreshToken)
		if err != nil {
			log.Error(ctx, domain.LogErrorUpdateTokensFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrRefreshTokens, err)
		}
		return &domain.TokenPair{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, ExpiresAt: response.ExpiresAt.AsTime()}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrRefreshTokens, err)
	}
	return result.(*domain.TokenPair), nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceLogout)
	err := s.resilience.ExecuteWithResilience(ctx, "Logout", func() error {
		err := s.authClient.Logout(ctx, refreshToken)
		if err != nil {
			log.Error(ctx, domain.LogErrorLogoutFailed, zap.Error(err))
			return fmt.Errorf("%w: %v", domain.ErrLogoutOperationFailed, err)
		}
		if refreshToken != "" {
			cacheKey := "profile:" + hashToken(refreshToken)
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := s.cache.Delete(cacheCtx, cacheKey); err != nil {
				log.Warn(ctx, domain.LogFailedToInvalidateProfileCache, zap.Error(err))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrLogoutOperationFailed, err)
	}
	return nil
}

func (s *NotesService) CreateNote(ctx context.Context, title, content string) (*domain.Note, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceCreateNote)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "CreateNote", func() (any, error) {
		response, err := s.notesClient.CreateNote(ctx, title, content)
		if err != nil {
			log.Error(ctx, domain.LogErrorCreateNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrFailedToCreateNote, err)
		}
		return convertNoteFromProto(response.Note), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrFailedToCreateNote, err)
	}
	return result.(*domain.Note), nil
}

func (s *NotesService) GetNote(ctx context.Context, noteID string) (*domain.Note, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceGetNote)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "GetNote", func() (any, error) {
		response, err := s.notesClient.GetNote(ctx, noteID)
		if err != nil {
			log.Error(ctx, domain.LogErrorGetNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrFailedToGetNote, err)
		}
		return convertNoteFromProto(response.Note), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrFailedToGetNote, err)
	}
	return result.(*domain.Note), nil
}

func (s *NotesService) ListNotes(ctx context.Context, limit, offset int32) ([]*domain.Note, int, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceListNotes)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "ListNotes", func() (any, error) {
		response, err := s.notesClient.ListNotes(ctx, limit, offset)
		if err != nil {
			log.Error(ctx, domain.LogErrorListNotesFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrFailedToListNotes, err)
		}
		notes := make([]*domain.Note, len(response.Notes))
		for i, note := range response.Notes {
			notes[i] = convertNoteFromProto(note)
		}
		return struct {
			notes []*domain.Note
			total int
		}{notes: notes, total: int(response.TotalCount)}, nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", domain.ErrFailedToListNotes, err)
	}
	re := result.(struct {
		notes []*domain.Note
		total int
	})
	return re.notes, re.total, nil
}

func (s *NotesService) UpdateNote(ctx context.Context, noteID string, title, content *string) (*domain.Note, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceUpdateNote)
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "UpdateNote", func() (any, error) {
		response, err := s.notesClient.UpdateNote(ctx, noteID, title, content)
		if err != nil {
			log.Error(ctx, domain.LogErrorUpdateNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%w: %v", domain.ErrFailedToUpdateNote, err)
		}
		return convertNoteFromProto(response.Note), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrFailedToUpdateNote, err)
	}
	return result.(*domain.Note), nil
}

func (s *NotesService) DeleteNote(ctx context.Context, noteID string) error {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceDeleteNote)
	err := s.resilience.ExecuteWithResilience(ctx, "DeleteNote", func() error {
		err := s.notesClient.DeleteNote(ctx, noteID)
		if err != nil {
			log.Error(ctx, domain.LogErrorDeleteNoteFailed, zap.Error(err))
			return fmt.Errorf("%w: %v", domain.ErrFailedToDeleteNote, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrFailedToDeleteNote, err)
	}
	return nil
}

func (s *AuthService) GetUserProfile(ctx context.Context) (*domain.User, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceGetProfile)
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
		tokenHash := hashToken(token)
		cacheKey := "profile:" + tokenHash
		cachedProfile, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cachedProfile != "" {
			var profile domain.User
			if err := json.Unmarshal([]byte(cachedProfile), &profile); err == nil {
				log.Debug(ctx, domain.LogUserProfileFoundInCache)
				return &profile, nil
			}
		}
	}
	result, err := fault.ExecuteWithResilienceResult(s.resilience, ctx, "GetUserProfile", func() (any, error) {
		profile, err := s.authClient.GetUserProfile(ctx)
		if err != nil {
			log.Error(ctx, domain.LogErrorGetProfileFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorGetProfileFailed, err)
		}
		profileDto := &domain.User{ID: profile.UserId, Email: profile.Email, Username: profile.Username, CreatedAt: profile.CreatedAt.AsTime()}
		if token != "" {
			tokenHash := hashToken(token)
			cacheKey := "profile:" + tokenHash
			profileJSON, err := json.Marshal(profileDto)
			if err == nil {
				cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				err := s.cache.Set(cacheCtx, cacheKey, string(profileJSON), 15*time.Minute)
				if err != nil {
					log.Warn(ctx, domain.LogFailedToCacheUserProfile, zap.Error(err))
				} else {
					log.Debug(ctx, domain.LogUserProfileCachedSuccessfully)
				}
			}
		}
		return profileDto, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrProfileRetrievalFailed, err)
	}
	return result.(*domain.User), nil
}

func hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func convertNoteFromProto(protoNote *notesv1.Note) *domain.Note {
	if protoNote == nil {
		return nil
	}
	return new(domain.Note{
		ID:        protoNote.NoteId,
		UserID:    protoNote.UserId,
		Title:     protoNote.Title,
		Content:   protoNote.Content,
		CreatedAt: protoNote.CreatedAt.AsTime(),
		UpdatedAt: protoNote.UpdatedAt.AsTime(),
	})
}
