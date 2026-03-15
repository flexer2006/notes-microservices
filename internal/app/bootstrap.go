package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
	grpcadapter "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	httpServer "github.com/flexer2006/notes-microservices/internal/adapters/http"
	jwtadapter "github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	postgresadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/adapters/redis"
	"github.com/flexer2006/notes-microservices/internal/config"
	dbpg "github.com/flexer2006/notes-microservices/internal/db/postgres"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	googlegrpc "google.golang.org/grpc"
)

func initLogger(cfg *config.Config) (*logger.Logger, error) {
	if cfg.Logging == nil {
		return nil, fmt.Errorf("logging config is missing")
	}
	logEnv := logger.Development
	if strings.ToLower(cfg.Logging.Mode) == "production" {
		logEnv = logger.Production
	}
	return logger.NewLogger(logEnv, cfg.Logging.Level)
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return fallback
}

func migrationSourceURL(service string) string {
	return fmt.Sprintf("file://migrations/%s", service)
}

func StartAuth(ctx context.Context, configPath string) error {
	cfg, err := Load(ctx, configPath)
	if err != nil {
		return err
	}
	log, err := initLogger(cfg)
	if err != nil {
		return err
	}
	logger.SetGlobalLogger(log)
	if cfg.Postgres == nil {
		return fmt.Errorf("postgres config is required")
	}
	if cfg.JWT == nil {
		return fmt.Errorf("jwt config is required")
	}
	if cfg.GRPC == nil {
		return fmt.Errorf("grpc config is required")
	}
	if cfg.Shutdown == nil {
		return fmt.Errorf("shutdown config is required")
	}
	dbCfg := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Database)
	if err := dbpg.Migrate(ctx, "postgres://"+cfg.Postgres.User+":"+cfg.Postgres.Password+"@"+cfg.Postgres.Host+":"+fmt.Sprint(cfg.Postgres.Port)+"/"+cfg.Postgres.Database+"?sslmode=disable", migrationSourceURL("auth")); err != nil {
		return err
	}
	database, err := dbpg.New(ctx, dbCfg, cfg.Postgres.MinConn, cfg.Postgres.MaxConn)
	if err != nil {
		return err
	}
	log.Info(ctx, domain.LogServiceStarted,
		zap.String("log_level", cfg.Logging.Level),
		zap.String("startup_time", time.Now().Format(time.RFC3339)))
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
	grpcServer.RegisterService(func(server *googlegrpc.Server) {
		authv1.RegisterAuthServiceServer(server, authHandler)
		authv1.RegisterUserServiceServer(server, userHandler)
	})
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

	if cfg.GRPCClient == nil {
		return fmt.Errorf("grpc client config is required")
	}
	if cfg.Redis == nil {
		return fmt.Errorf("redis config is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("http config is required")
	}
	if cfg.Shutdown == nil {
		return fmt.Errorf("shutdown config is required")
	}
	log.Info(ctx, domain.LogServiceStarted,
		zap.String("log_level", cfg.Logging.Level),
		zap.String("startup_time", time.Now().Format(time.RFC3339)))
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
			log.Error(ctx, domain.LogErrStartHTTPServer, zap.Error(err))
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
	cfg, err := Load(ctx, configPath)
	if err != nil {
		return err
	}
	log, err := initLogger(cfg)
	if err != nil {
		return err
	}
	logger.SetGlobalLogger(log)
	if cfg.Postgres == nil {
		return fmt.Errorf("postgres config is required")
	}
	if cfg.JWT == nil {
		return fmt.Errorf("jwt config is required")
	}
	if cfg.GRPC == nil {
		return fmt.Errorf("grpc config is required")
	}
	if cfg.Shutdown == nil {
		return fmt.Errorf("shutdown config is required")
	}
	dbCfg := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Database)
	if err := dbpg.Migrate(ctx, "postgres://"+cfg.Postgres.User+":"+cfg.Postgres.Password+"@"+cfg.Postgres.Host+":"+fmt.Sprint(cfg.Postgres.Port)+"/"+cfg.Postgres.Database+"?sslmode=disable", migrationSourceURL("notes")); err != nil {
		return err
	}
	database, err := dbpg.New(ctx, dbCfg, cfg.Postgres.MinConn, cfg.Postgres.MaxConn)
	if err != nil {
		return err
	}
	log.Info(ctx, domain.LogServiceStarted,
		zap.String("log_level", cfg.Logging.Level),
		zap.String("startup_time", time.Now().Format(time.RFC3339)))
	repoFactory := postgresadapter.NewRepositoryFactory(database.Pool())
	noteRepo := repoFactory.NoteRepository()
	accessTTL := parseDurationOrDefault(cfg.JWT.AccessTokenTTL, 15*time.Minute)
	refreshTTL := parseDurationOrDefault(cfg.JWT.RefreshTokenTTL, 24*time.Hour)
	tokenService := jwtadapter.New(cfg.JWT.SecretKey, accessTTL, refreshTTL)
	noteUseCase := NewNoteUseCase(noteRepo, tokenService)
	noteHandler := grpcadapter.NewNoteHandler(noteUseCase)
	grpcServer := grpcadapter.New(cfg)
	grpcServer.RegisterService(func(server *googlegrpc.Server) {
		notesv1.RegisterNoteServiceServer(server, noteHandler)
	})
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
