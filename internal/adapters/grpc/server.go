package grpc

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

type Server struct {
	cfg          *config.Config
	server       *grpc.Server
	healthServer *health.Server
	serveErr     chan error
}

const (
	defaultInterceptorCount = 2
	maxMessageBytes         = 1 << 20
)

func New(cfg *config.Config, opts ...grpc.ServerOption) *Server {
	defaultOpts := make([]grpc.ServerOption, 0, defaultInterceptorCount+2+len(opts))
	defaultOpts = append(defaultOpts,
		grpc.ChainUnaryInterceptor(unaryRequestIDInterceptor),
		grpc.ChainStreamInterceptor(streamRequestIDInterceptor),
		grpc.MaxRecvMsgSize(maxMessageBytes),
		grpc.MaxSendMsgSize(maxMessageBytes),
	)
	defaultOpts = append(defaultOpts, opts...)
	server := grpc.NewServer(defaultOpts...)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)

	return new(Server{
		cfg:          cfg,
		server:       server,
		healthServer: healthServer,
		serveErr:     make(chan error, 1),
	})
}

func (s *Server) Start(ctx context.Context) error {
	log := logger.Log(ctx)
	address := net.JoinHostPort(s.cfg.GRPC.Host, strconv.Itoa(s.cfg.GRPC.Port))

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("start grpc server on %s: %w", address, err)
	}

	if s.reflectionEnabled() {
		reflection.Register(s.server)
		log.Info(ctx, "gRPC reflection enabled")
	}

	go func() {
		s.serveErr <- s.server.Serve(listener)
	}()

	log.Info(ctx, "gRPC server started", zap.String("address", address))

	return nil
}

func (s *Server) Err() <-chan error {
	return s.serveErr
}

func (s *Server) Stop(ctx context.Context) {
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	}

	stopped := make(chan struct{})

	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-ctx.Done():
		s.server.Stop()
		<-stopped
	}
}

func (s *Server) RegisterService(registerFn func(server grpc.ServiceRegistrar)) {
	registerFn(s.server)
}

func (s *Server) reflectionEnabled() bool {
	if s.cfg == nil || s.cfg.GRPC == nil {
		return false
	}

	return s.cfg.GRPC.Reflection
}
