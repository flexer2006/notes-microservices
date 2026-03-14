package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	"github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
	"github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	services "github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	"github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/config"
	dbpg "github.com/flexer2006/notes-microservices/internal/db/postgres"
	"github.com/flexer2006/notes-microservices/internal/di"
	app "github.com/flexer2006/notes-microservices/internal/di/app"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
	googlegrpc "google.golang.org/grpc"
)

func main() {
	env := logger.Development
	if strings.ToLower(os.Getenv(domain.LogEnvLoggerMode)) == "production" {
		env = logger.Production
	}
	log, err := logger.NewLogger(env, os.Getenv(domain.LogEnvLoggerLevel))
	if err != nil {
		panic(domain.LogErrInitLogger + ": " + err.Error())
	}
	logger.SetGlobalLogger(log)
	ctx := logger.NewRequestIDContext(context.Background(), "")
	var exitCode int
	func() {
		defer func() {
			if err := log.Sync(); err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, domain.LogErrSyncStderr) || strings.Contains(errMsg, domain.LogErrSyncStdout) {
					return
				}
				if _, writeErr := fmt.Fprintf(os.Stderr, "%s: %v\n", domain.LogErrSyncLogger, err); writeErr != nil {
					panic(writeErr)
				}
			}
		}()
		cfg, err := config.Load(ctx)
		if err != nil {
			log.Error(ctx, domain.LogErrLoadConfig, zap.Error(err))
			exitCode = 1
			return
		}
		finalLogger, err := logger.NewLogger(cfg.Logging.GetEnvironment(), cfg.Logging.Level)
		if err != nil {
			log.Error(ctx, domain.LogErrInitLoggerWithConfig, zap.Error(err))
			exitCode = 1
			return
		}
		logger.SetGlobalLogger(finalLogger)
		database, err := dbpg.New(ctx, cfg.Postgres.GetDSN(), cfg.Postgres.MinConn, cfg.Postgres.MaxConn)
		if err != nil {
			log.Error(ctx, domain.LogErrInitDB, zap.Error(err))
			exitCode = 1
			return
		}
		log.Info(ctx, domain.LogServiceStarted,
			zap.String("environment", string(env)),
			zap.String("log_level", cfg.Logging.Level),
			zap.String("startup_time", time.Now().Format(time.RFC3339)))
		log.Info(ctx, domain.LogInitRepo)
		repoFactory := postgres.NewAuthRepositoryFactory(database.Pool())
		userRepo := repoFactory.UserRepository()
		tokenRepo := repoFactory.TokenRepository()
		log.Info(ctx, domain.LogInitServices)
		passwordService := bcrypt.NewBcrypt(cfg.JWT.BCryptCost)
		tokenService := services.NewJWT(cfg.JWT.SecretKey, cfg.JWT.GetAccessTokenTTL(), cfg.JWT.GetRefreshTokenTTL())
		log.Info(ctx, domain.LogInitUseCases)
		authUseCase := app.NewAuthUseCase(userRepo, tokenRepo, passwordService, tokenService)
		userUseCase := app.NewUserUseCase(userRepo)
		log.Info(ctx, domain.LogInitHandlers)
		authHandler := grpc.NewAuthHandler(authUseCase)
		userHandler := grpc.NewUserHandler(userUseCase, tokenService)
		log.Info(ctx, domain.LogInitGRPCServer)
		grpcServer := grpc.New(&cfg.GRPC)
		grpcServer.RegisterService(func(server *googlegrpc.Server) {
			authv1.RegisterAuthServiceServer(server, authHandler)
			authv1.RegisterUserServiceServer(server, userHandler)
		})
		log.Info(ctx, domain.LogStartingGRPC)
		if err := grpcServer.Start(ctx); err != nil {
			log.Error(ctx, domain.LogErrStartGRPC, zap.Error(err))
			exitCode = 1
			return
		}
		di.Wait(ctx, cfg.Shutdown.GetTimeout(),
			func(ctx context.Context) error {
				log.Info(ctx, domain.LogClosingDB)
				database.Close(ctx)
				return nil
			},
			func(ctx context.Context) error {
				log.Info(ctx, domain.LogStoppingGRPC)
				grpcServer.Stop(ctx)
				return nil
			},
		)
		log.Info(ctx, domain.LogServiceShutdownDone)
	}()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
