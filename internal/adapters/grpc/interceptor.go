package grpc

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

//nolint:containedctx
type serverStreamWithContext struct {
	grpc.ServerStream

	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context {
	return s.ctx
}

func NewUnaryAuthInterceptor(
	tokenSvc ports.TokenService,
	protectedMethods ...string,
) grpc.UnaryServerInterceptor {
	protected := make(map[string]struct{}, len(protectedMethods))
	for _, m := range protectedMethods {
		protected[m] = struct{}{}
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if _, ok := protected[info.FullMethod]; !ok {
			return handler(ctx, req)
		}

		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, domain.ErrUnauthorized.Error())
		}

		userID, err := tokenSvc.ValidateAccessToken(ctx, token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, domain.ErrUnauthorized.Error())
		}

		return handler(authctx.WithUserID(ctx, userID), req)
	}
}

func userIDFromContext(ctx context.Context) (string, error) {
	if userID := authctx.UserIDFrom(ctx); userID != "" {
		return userID, nil
	}

	return "", status.Error(codes.Unauthenticated, domain.ErrUnauthorized.Error())
}

func unaryRequestIDInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
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

func streamRequestIDInterceptor(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
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
