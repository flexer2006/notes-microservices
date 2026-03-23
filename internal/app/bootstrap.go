package app

import (
	"context"
	"fmt"
	"time"

	"github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
	grpcadapter "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	httpServer "github.com/flexer2006/notes-microservices/internal/adapters/http"
	jwtadapter "github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	postgresadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/adapters/redis"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

func StartAuth(ctx context.Context, configPath string) error {
	cfg, err := initService(ctx, configPath)
	if err != nil {
		return err
	}
	database, err := setupPostgres(ctx, cfg, "auth")
	if err != nil {
		return err
	}
	serviceStartupLog(ctx, cfg, "authentication")
	repoFactory := postgresadapter.NewAuthRepositoryFactory(database.Pool())
	passwordSvc := bcrypt.NewBcrypt(cfg.JWT.BCryptCost)
	accessTTL := parseDurationOrDefault(cfg.JWT.AccessTokenTTL, 15*time.Minute)
	refreshTTL := parseDurationOrDefault(cfg.JWT.RefreshTokenTTL, 24*time.Hour)
	tokenSvc := jwtadapter.New(cfg.JWT.SecretKey, accessTTL, refreshTTL)
	authUC := NewAuthUseCase(repoFactory.UserRepository(), repoFactory.TokenRepository(), passwordSvc, tokenSvc)
	userUC := NewUserUseCase(repoFactory.UserRepository())
	authHandler := grpcadapter.NewAuthHandler(authUC)
	userHandler := grpcadapter.NewUserHandler(userUC, tokenSvc)
	grpcServer := grpcadapter.New(cfg)
	grpcServer.RegisterService(authHandler.RegisterService)
	grpcServer.RegisterService(userHandler.RegisterService)
	if err := grpcServer.Start(ctx); err != nil {
		return err
	}
	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second,
		func(ctx context.Context) error { database.Close(ctx); return nil },
		func(ctx context.Context) error { grpcServer.Stop(ctx); return nil },
	)
}

func StartGateway(ctx context.Context, configPath string) error {
	cfg, err := Load(ctx, configPath)
	if err != nil {
		return err
	}
	log, err := initLogger(cfg)
	if err != nil {
		return err
	}
	logger.SetGlobalLogger(log)

	if err := requireGatewayConfig(cfg); err != nil {
		return err
	}
	serviceStartupLog(ctx, cfg, "gateway")
	authClient, err := grpcadapter.NewAuthClient(ctx, cfg)
	if err != nil {
		return err
	}
	notesClient, err := grpcadapter.NewNotesClient(ctx, cfg)
	if err != nil {
		_ = authClient.Close()
		return err
	}
	redisCache, err := redis.NewRedisCache(ctx, cfg)
	if err != nil {
		_ = notesClient.Close()
		_ = authClient.Close()
		return err
	}
	authService := NewAuthService(authClient, redisCache)
	notesService := NewNotesService(notesClient, redisCache)
	app := fiber.New(fiber.Config{ReadTimeout: cfg.HTTP.ReadTimeout, WriteTimeout: cfg.HTTP.WriteTimeout})
	httpServer.SetupRouter(app, authService, notesService)
	go func() {
		if err := app.Listen(fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)); err != nil {
			log.Error(ctx, domain.ErrFailedToServeRequest.Error(), zap.Error(err))
		}
	}()
	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second,
		func(_ context.Context) error { return authClient.Close() },
		func(_ context.Context) error { return notesClient.Close() },
		func(_ context.Context) error { return redisCache.Close() },
		func(_ context.Context) error { return app.Shutdown() },
	)
}

func StartNotes(ctx context.Context, configPath string) error {
	cfg, err := initService(ctx, configPath)
	if err != nil {
		return err
	}
	database, err := setupPostgres(ctx, cfg, "notes")
	if err != nil {
		return err
	}
	serviceStartupLog(ctx, cfg, "notes")
	repoFactory := postgresadapter.NewRepositoryFactory(database.Pool())
	noteRepo := repoFactory.NoteRepository()
	accessTTL := parseDurationOrDefault(cfg.JWT.AccessTokenTTL, 15*time.Minute)
	refreshTTL := parseDurationOrDefault(cfg.JWT.RefreshTokenTTL, 24*time.Hour)
	tokenService := jwtadapter.New(cfg.JWT.SecretKey, accessTTL, refreshTTL)
	noteUseCase := NewNoteUseCase(noteRepo, tokenService)
	noteHandler := grpcadapter.NewNoteHandler(noteUseCase)
	grpcServer := grpcadapter.New(cfg)
	grpcServer.RegisterService(noteHandler.RegisterService)
	if err := grpcServer.Start(ctx); err != nil {
		return err
	}
	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second,
		func(ctx context.Context) error {
			database.Close(ctx)
			return nil
		},
		func(ctx context.Context) error {
			grpcServer.Stop(ctx)
			return nil
		},
	)
}
