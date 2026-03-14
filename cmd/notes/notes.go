package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	grpcHandler "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
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
	"google.golang.org/grpc/reflection"
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
		repoFactory := postgres.NewRepositoryFactory(database.Pool())
		noteRepo := repoFactory.NoteRepository()
		log.Info(ctx, domain.LogInitServices)
		tokenService := services.NewJWT(cfg.JWT.SecretKey, cfg.JWT.GetAccessTokenTTL(), cfg.JWT.GetRefreshTokenTTL())
		log.Info(ctx, domain.LogInitUseCases)
		noteUseCase := app.NewNoteUseCase(noteRepo, tokenService)
		log.Info(ctx, domain.LogInitHandlers)
		noteHandler := grpcHandler.NewNoteHandler(noteUseCase)
		log.Info(ctx, domain.LogInitGRPCServer)
		grpcServer := New(&cfg.GRPC)

		grpcServer.RegisterService(func(server *googlegrpc.Server) {
			notesv1.RegisterNoteServiceServer(server, noteHandler)
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

type Server struct {
	server   *googlegrpc.Server
	address  string
	listener net.Listener
}

func New(config *config.GRPCConfig) *Server {
	address := config.GetAddress()
	return &Server{
		server:  googlegrpc.NewServer(),
		address: address,
	}
}

func (s *Server) RegisterService(registerFunc func(*googlegrpc.Server)) {
	registerFunc(s.server)
	reflection.Register(s.server)
}

func (s *Server) Start(ctx context.Context) error {
	log := logger.Log(ctx)

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	log.Info(ctx, "gRPC server started", zap.String("address", s.address))

	go func() {
		if err := s.server.Serve(listener); err != nil {
			log.Error(ctx, "failed to serve gRPC", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) Stop(ctx context.Context) {
	log := logger.Log(ctx)
	log.Info(ctx, "stopping gRPC server")

	s.server.GracefulStop()
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Error(ctx, "failed to close listener", zap.Error(err))
		}
	}
}
