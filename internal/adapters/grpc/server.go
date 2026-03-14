package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Server struct {
	cfg    *config.Config
	server *grpc.Server
}

func New(cfg *config.Config) *Server {
	return new(Server{
		cfg:    cfg,
		server: grpc.NewServer(),
	})
}

func (s *Server) Start(ctx context.Context) error {
	log := logger.Log(ctx)
	address := net.JoinHostPort(s.cfg.GRPC.Host, fmt.Sprint(s.cfg.GRPC.Port))
	log.Info(ctx, domain.LogServerStarting, zap.String("address", address))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Error(ctx, domain.LogErrServerStart, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.LogErrServerStart, err)
	}
	reflection.Register(s.server)
	go func() {
		if err := s.server.Serve(listener); err != nil {
			log.Error(ctx, domain.LogErrServerStart, zap.Error(err))
		}
	}()
	log.Info(ctx, domain.LogServerStarted, zap.String("address", address))
	return nil
}

func (s *Server) Stop(ctx context.Context) {
	logger.Log(ctx).Info(ctx, domain.LogServerStopping)
	s.server.GracefulStop()
}

func (s *Server) RegisterService(registerFn func(server *grpc.Server)) {
	registerFn(s.server)
}

func (s *Server) RegisterGRPCService(desc *grpc.ServiceDesc, impl any) {
	s.server.RegisterService(desc, impl)
}
