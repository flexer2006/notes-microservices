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
	"github.com/flexer2006/notes-microservices/internal/ports"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type AuthServiceImpl struct {
	authClient ports.AuthServiceClient
	cache      ports.Cache
	resilience *fault.ServiceResilience
}

func NewAuthService(authClient ports.AuthServiceClient, cache ports.Cache) ports.AuthService {
	return new(AuthServiceImpl{
		authClient: authClient,
		cache:      cache,
		resilience: fault.NewServiceResilience("auth-service"),
	})
}

func (s *AuthServiceImpl) Register(ctx context.Context, req *ports.RegisterRequest) (*ports.TokenResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceRegister)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "Register", func() (any, error) {
		response, err := s.authClient.Register(ctx, req.Email, req.Username, req.Password)
		if err != nil {
			log.Error(ctx, domain.LogErrorRegisterFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorRegisterFailed, err)
		}
		return new(ports.TokenResponse{
			UserID:       response.UserId,
			AccessToken:  response.AccessToken,
			RefreshToken: response.RefreshToken,
			ExpiresAt:    response.ExpiresAt.AsTime(),
		}), nil
	})
	if err != nil {
		return nil, fmt.Errorf("user registration failed: %w", err)
	}
	return result.(*ports.TokenResponse), nil
}

func (s *AuthServiceImpl) Login(ctx context.Context, req *ports.LoginRequest) (*ports.TokenResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceLogin)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "Login", func() (any, error) {
		response, err := s.authClient.Login(ctx, req.Email, req.Password)
		if err != nil {
			log.Error(ctx, domain.LogErrorLoginFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorLoginFailed, err)
		}
		return new(ports.TokenResponse{
			UserID:       response.UserId,
			Username:     response.Username,
			AccessToken:  response.AccessToken,
			RefreshToken: response.RefreshToken,
			ExpiresAt:    response.ExpiresAt.AsTime(),
		}), nil
	})
	if err != nil {
		return nil, fmt.Errorf("user login failed: %w", err)
	}
	return result.(*ports.TokenResponse), nil
}

func (s *AuthServiceImpl) RefreshTokens(ctx context.Context, req *ports.RefreshRequest) (*ports.TokenResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceTokenRefresh)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "RefreshTokens", func() (any, error) {
		response, err := s.authClient.RefreshTokens(ctx, req.RefreshToken)
		if err != nil {
			log.Error(ctx, domain.LogErrorUpdateTokensFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorUpdateTokensFailed, err)
		}
		return new(ports.TokenResponse{
			AccessToken:  response.AccessToken,
			RefreshToken: response.RefreshToken,
			ExpiresAt:    response.ExpiresAt.AsTime(),
		}), nil
	})
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	return result.(*ports.TokenResponse), nil
}

func (s *AuthServiceImpl) Logout(ctx context.Context, req *ports.LogoutRequest) error {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceLogout)
	err := s.resilience.ExecuteWithResilience(ctx, "Logout", func() error {
		err := s.authClient.Logout(ctx, req.RefreshToken)
		if err != nil {
			log.Error(ctx, domain.LogErrorLogoutFailed, zap.Error(err))
			return fmt.Errorf("%s: %w", domain.LogErrorLogoutFailed, err)
		}
		if req.RefreshToken != "" {
			tokenHash := hashToken(req.RefreshToken)
			cacheKey := "profile:" + tokenHash
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := s.cache.Delete(cacheCtx, cacheKey); err != nil {
				log.Warn(ctx, "Failed to invalidate profile cache", zap.Error(err))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("user logout failed: %w", err)
	}
	return nil
}

type NotesServiceImpl struct {
	notesClient ports.NotesServiceClient
	cache       ports.Cache
	resilience  *fault.ServiceResilience
}

func NewNotesService(notesClient ports.NotesServiceClient, cache ports.Cache) ports.NotesService {
	return new(NotesServiceImpl{
		notesClient: notesClient,
		cache:       cache,
		resilience:  fault.NewServiceResilience("notes-service"),
	})
}

func (s *NotesServiceImpl) CreateNote(ctx context.Context, req *ports.CreateNoteRequest) (*ports.NoteResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceCreateNote)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "CreateNote", func() (any, error) {
		response, err := s.notesClient.CreateNote(ctx, req.Title, req.Content)
		if err != nil {
			log.Error(ctx, domain.LogErrorCreateNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorCreateNoteFailed, err)
		}
		return convertNoteResponseFromProto(response), nil
	})
	if err != nil {
		return nil, fmt.Errorf("note creation failed: %w", err)
	}
	return result.(*ports.NoteResponse), nil
}

func (s *NotesServiceImpl) GetNote(ctx context.Context, noteID string) (*ports.NoteResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceGetNote)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "GetNote", func() (any, error) {
		response, err := s.notesClient.GetNote(ctx, noteID)
		if err != nil {
			log.Error(ctx, domain.LogErrorGetNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorGetNoteFailed, err)
		}
		return convertNoteResponseFromProto(response), nil
	})
	if err != nil {
		return nil, fmt.Errorf("get note failed: %w", err)
	}
	return result.(*ports.NoteResponse), nil
}

func (s *NotesServiceImpl) ListNotes(ctx context.Context, limit, offset int32) (*ports.ListNotesResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceListNotes)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "ListNotes", func() (any, error) {
		response, err := s.notesClient.ListNotes(ctx, limit, offset)
		if err != nil {
			log.Error(ctx, domain.LogErrorListNotesFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorListNotesFailed, err)
		}
		return convertListNotesResponseFromProto(response), nil
	})
	if err != nil {
		return nil, fmt.Errorf("list notes failed: %w", err)
	}
	return result.(*ports.ListNotesResponse), nil
}

func (s *NotesServiceImpl) UpdateNote(ctx context.Context, noteID string, req *ports.UpdateNoteRequest) (*ports.NoteResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceUpdateNote)
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "UpdateNote", func() (any, error) {
		response, err := s.notesClient.UpdateNote(ctx, noteID, req.Title, req.Content)
		if err != nil {
			log.Error(ctx, domain.LogErrorUpdateNoteFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorUpdateNoteFailed, err)
		}
		return convertNoteResponseFromProto(response), nil
	})
	if err != nil {
		return nil, fmt.Errorf("update note failed: %w", err)
	}
	return result.(*ports.NoteResponse), nil
}

func (s *NotesServiceImpl) DeleteNote(ctx context.Context, noteID string) error {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogServiceDeleteNote)
	err := s.resilience.ExecuteWithResilience(ctx, "DeleteNote", func() error {
		err := s.notesClient.DeleteNote(ctx, noteID)
		if err != nil {
			log.Error(ctx, domain.LogErrorDeleteNoteFailed, zap.Error(err))
			return fmt.Errorf("%s: %w", domain.LogErrorDeleteNoteFailed, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete note failed: %w", err)
	}
	return nil
}

func convertNoteFromProto(protoNote *notesv1.Note) *ports.Note {
	if protoNote == nil {
		return nil
	}
	return new(ports.Note{
		ID:        protoNote.NoteId,
		UserID:    protoNote.UserId,
		Title:     protoNote.Title,
		Content:   protoNote.Content,
		CreatedAt: protoNote.CreatedAt.AsTime(),
		UpdatedAt: protoNote.UpdatedAt.AsTime(),
	})
}

func convertNoteResponseFromProto(protoResp *notesv1.NoteResponse) *ports.NoteResponse {
	if protoResp == nil {
		return nil
	}
	return new(ports.NoteResponse{
		Note: convertNoteFromProto(protoResp.Note),
	})
}

func convertListNotesResponseFromProto(protoResp *notesv1.ListNotesResponse) *ports.ListNotesResponse {
	if protoResp == nil {
		return nil
	}
	notes := make([]*ports.Note, len(protoResp.Notes))
	for i, note := range protoResp.Notes {
		notes[i] = convertNoteFromProto(note)
	}
	return new(ports.ListNotesResponse{
		Notes:      notes,
		TotalCount: protoResp.TotalCount,
		Offset:     protoResp.Offset,
		Limit:      protoResp.Limit,
	})
}

func (s *AuthServiceImpl) GetUserProfile(ctx context.Context) (*ports.UserProfileResponse, error) {
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
			var profile ports.UserProfileResponse
			if err := json.Unmarshal([]byte(cachedProfile), &profile); err == nil {
				log.Debug(ctx, "User profile found in cache")
				return &profile, nil
			}
		}
	}
	result, err := s.resilience.ExecuteWithResultTokenResponse(ctx, "GetUserProfile", func() (any, error) {
		profile, err := s.authClient.GetUserProfile(ctx)
		if err != nil {
			log.Error(ctx, domain.LogErrorGetProfileFailed, zap.Error(err))
			return nil, fmt.Errorf("%s: %w", domain.LogErrorGetProfileFailed, err)
		}
		profileDto := new(ports.UserProfileResponse{
			UserID:    profile.UserId,
			Email:     profile.Email,
			Username:  profile.Username,
			CreatedAt: profile.CreatedAt.AsTime(),
		})
		if token != "" {
			tokenHash := hashToken(token)
			cacheKey := "profile:" + tokenHash
			profileJSON, err := json.Marshal(profileDto)
			if err == nil {
				cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				err := s.cache.Set(cacheCtx, cacheKey, string(profileJSON), 15*time.Minute)
				if err != nil {
					log.Warn(ctx, "Failed to cache user profile", zap.Error(err))
				} else {
					log.Debug(ctx, "User profile cached successfully")
				}
			}
		}
		return profileDto, nil
	})
	if err != nil {
		return nil, fmt.Errorf("profile retrieval failed: %w", err)
	}
	return result.(*ports.UserProfileResponse), nil
}

func hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return fmt.Sprintf("%x", h.Sum(nil))
}
