package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/ilyakaznacheev/cleanenv"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type AuthUseCase struct {
	userRepo    ports.UserRepository
	tokenRepo   ports.TokenRepository
	passwordSvc ports.PasswordService
	tokenSvc    ports.TokenService
}

type AuthService struct {
	authClient AuthServiceClient
	cache      ports.Cache
	resilience *fault.ServiceResilience
}

type NotesService struct {
	notesClient NotesServiceClient
	cache       ports.Cache
	resilience  *fault.ServiceResilience
}

type AuthServiceClient interface {
	Register(ctx context.Context, email, username, password string) (*authv1.RegisterResponse, error)
	Login(ctx context.Context, email, password string) (*authv1.LoginResponse, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*authv1.RefreshTokensResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	GetUserProfile(ctx context.Context) (*authv1.UserProfileResponse, error)
}

type NotesServiceClient interface {
	CreateNote(ctx context.Context, title, content string) (*notesv1.NoteResponse, error)
	UpdateNote(ctx context.Context, noteID string, title, content *string) (*notesv1.NoteResponse, error)
	ListNotes(ctx context.Context, limit, offset int32) (*notesv1.ListNotesResponse, error)
	GetNote(ctx context.Context, noteID string) (*notesv1.NoteResponse, error)
	DeleteNote(ctx context.Context, noteID string) error
}

func Wait(ctx context.Context, timeout time.Duration, hooks ...func(context.Context) error) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-sigCtx.Done():
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	g, gctx := errgroup.WithContext(shutdownCtx)
	for _, hook := range hooks {
		g.Go(func() error {
			return hook(gctx)
		})
	}
	return g.Wait()
}

func Load(ctx context.Context, configPath string) (*config.Config, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogLoadingConfig)
	cfg := new(config.Config)
	if configPath != "" {
		if fi, err := os.Stat(configPath); err == nil && !fi.IsDir() {
			if err := cleanenv.ReadConfig(configPath, cfg); err != nil {
				log.Error(ctx, "config read failed", zap.Error(err), zap.String("path", configPath))
				return nil, fmt.Errorf("read config %s: %w", configPath, err)
			}
		}
	}
	if err := cleanenv.ReadEnv(cfg); err != nil {
		log.Error(ctx, "env read failed", zap.Error(err))
		return nil, fmt.Errorf("read env: %w", err)
	}
	if loggable, ok := any(cfg).(interface{ LogFields() []zap.Field }); ok {
		log.Info(ctx, "configuration loaded", loggable.LogFields()...)
	} else {
		log.Info(ctx, "configuration loaded")
	}
	return cfg, nil
}
