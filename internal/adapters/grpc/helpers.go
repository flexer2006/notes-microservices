package grpc

import (
	"context"
	"strings"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", domain.ErrMetadataNotFound
	}
	logger.Log(ctx).Debug(ctx, domain.LogReceivedMetadata, zap.Any("metadata", md))
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", domain.ErrAuthHeaderNotFound
	}
	authHeader := values[0]
	logger.Log(ctx).Debug(ctx, domain.LogReceivedAuthorizationHeader, zap.String("auth_header", authHeader))
	if after, ok0 := strings.CutPrefix(authHeader, "Bearer "); ok0 {
		return after, nil
	}
	return authHeader, nil
}

func formatAuthorizationToken(token string) string {
	if token == "" {
		return ""
	}
	if !strings.HasPrefix(token, "Bearer ") {
		return "Bearer " + token
	}
	return token
}

func outgoingContextWithAuth(ctx context.Context) context.Context {
	token, err := extractBearerToken(ctx)
	if err != nil || token == "" {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formatAuthorizationToken(token)))
}
