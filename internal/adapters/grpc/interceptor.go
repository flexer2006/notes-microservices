package grpc

import (
	"context"

	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context {
	return s.ctx
}

func unaryRequestIDInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx = withRequestID(ctx)
	log := logger.Method(ctx, info.FullMethod).With(zap.String("middleware", "grpc-unary"))
	log.Debug(ctx, "gRPC unary request started")
	resp, err := handler(ctx, req)
	if err != nil {
		log.Error(ctx, "gRPC unary request failed", zap.Error(err))
	} else {
		log.Debug(ctx, "gRPC unary request completed")
	}
	return resp, err
}

func streamRequestIDInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := withRequestID(ss.Context())
	wrapped := new(serverStreamWithContext{ServerStream: ss, ctx: ctx})
	log := logger.Method(ctx, info.FullMethod).With(zap.String("middleware", "grpc-stream"))
	log.Debug(ctx, "gRPC stream request started")
	err := handler(srv, wrapped)
	if err != nil {
		log.Error(ctx, "gRPC stream request failed", zap.Error(err))
	} else {
		log.Debug(ctx, "gRPC stream request completed")
	}
	return err
}
