package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	grpcclient "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	httpServer "github.com/flexer2006/notes-microservices/internal/adapters/http"
	"github.com/flexer2006/notes-microservices/internal/adapters/redis"
	config "github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/di"
	services "github.com/flexer2006/notes-microservices/internal/di/app"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
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
		log = finalLogger
		log.Info(ctx, domain.LogServiceStarted,
			zap.String("environment", string(env)),
			zap.String("log_level", cfg.Logging.Level),
			zap.String("startup_time", time.Now().Format(time.RFC3339)))

		log.Info(ctx, domain.LogInitClients)
		authClient, err := grpcclient.NewAuthClient(ctx, &cfg.GRPCClient)
		if err != nil {
			log.Error(ctx, domain.LogErrCreateAuthClient, zap.Error(err))
			exitCode = 1
			return
		}
		notesClient, err := grpcclient.NewNotesClient(ctx, &cfg.GRPCClient)
		if err != nil {
			log.Error(ctx, domain.LogErrCreateNotesClient, zap.Error(err))
			exitCode = 1
			return
		}
		log.Info(ctx, domain.LogInitCache)
		redisCache, err := redis.NewRedisCache(ctx, &cfg.Redis)
		if err != nil {
			log.Error(ctx, domain.LogErrCreateRedisClient, zap.Error(err))
			exitCode = 1
			return
		}
		log.Info(ctx, domain.LogInitServices)
		authService := services.NewAuthService(authClient, redisCache)
		notesService := services.NewNotesService(notesClient, redisCache)
		log.Info(ctx, domain.LogInitHTTPServer)
		app := fiber.New(fiber.Config{
			ReadTimeout:  cfg.HTTP.ReadTimeout,
			WriteTimeout: cfg.HTTP.WriteTimeout,
		})
		httpServer.SetupRouter(app, authService, notesService)
		log.Info(ctx, domain.LogStartingHTTP, zap.String("address", cfg.HTTP.GetAddress()))
		go func() {
			if err := app.Listen(cfg.HTTP.GetAddress()); err != nil {
				log.Error(ctx, domain.LogErrStartHTTPServer, zap.Error(err))
			}
		}()
		di.Wait(ctx, cfg.Shutdown.GetTimeout(),
			func(ctx context.Context) error {
				if closer, ok := authClient.(interface{ Close() error }); ok {
					log.Info(ctx, "Closing auth client")
					if err := closer.Close(); err != nil {
						log.Warn(ctx, "Error closing auth client", zap.Error(err))
					}
				}
				return nil
			},
			func(ctx context.Context) error {
				if closer, ok := notesClient.(interface{ Close() error }); ok {
					log.Info(ctx, "Closing notes client")
					if err := closer.Close(); err != nil {
						log.Warn(ctx, "Error closing notes client", zap.Error(err))
					}
				}
				return nil
			},
			func(ctx context.Context) error {
				log.Info(ctx, "Closing Redis connection")
				return redisCache.Close()
			},
			func(ctx context.Context) error {
				log.Info(ctx, domain.LogStoppingHTTP)
				return app.Shutdown()
			},
		)
		log.Info(ctx, domain.LogServiceShutdownDone)
	}()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
