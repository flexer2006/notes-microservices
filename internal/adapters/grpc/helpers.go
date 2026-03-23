package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", domain.ErrMetadataNotFound
	}
	logger.Log(ctx).Debug(ctx, "Received metadata", zap.Any("metadata", md))
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", domain.ErrAuthHeaderNotFound
	}
	authHeader := values[0]
	logger.Log(ctx).Debug(ctx, "Received authorization header", zap.String("auth_header", authHeader))
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
	md := metadata.Pairs("authorization", formatAuthorizationToken(token))
	if requestID := logger.RequestIDFromContext(ctx); requestID != "" {
		md.Set("x-request-id", requestID)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func grpcErrorFromDomain(err error) error {
	if errors.Is(err, domain.ErrUnauthorized) {
		return status.Error(codes.Unauthenticated, domain.ErrInvalidOrExpiredToken.Error())
	}
	if errors.Is(err, domain.ErrNoteNotFound) {
		return status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
	}
	return status.Error(codes.Internal, domain.ErrInternalService.Error())
}

func authErrorFromDomain(err error) error {
	if errors.Is(err, domain.ErrUserAlreadyExists) {
		return fmt.Errorf("%s: %w", domain.ErrUserRegistrationFailed.Error(), domain.ErrUserAlreadyExists)
	}
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return fmt.Errorf("%s: %w", domain.ErrAuthenticationFailed.Error(), domain.ErrInvalidCredentials)
	}
	if errors.Is(err, domain.ErrInvalidRefreshToken) {
		return fmt.Errorf("%s: %w", domain.ErrTokenRefreshFailed.Error(), domain.ErrInvalidRefreshToken)
	}
	if errors.Is(err, domain.ErrNoteNotFound) {
		return fmt.Errorf("%s: %w", domain.ErrProfileRetrievalFailed.Error(), domain.ErrNoteNotFound)
	}
	return fmt.Errorf("%s: %w", domain.ErrAuthServiceError.Error(), domain.ErrAuthServiceInternal)
}

func newClientConn(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	if err := waitUntilReady(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func waitUntilReady(ctx context.Context, conn *grpc.ClientConn) error {
	for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
		if !conn.WaitForStateChange(ctx, state) {
			return domain.ErrAuthServiceConnectionTimeout
		}
	}
	return nil
}

func noteToProto(note *domain.Note) *notesv1.Note {
	if note == nil {
		return nil
	}
	return new(notesv1.Note{
		NoteId:    note.ID,
		UserId:    note.UserID,
		Title:     note.Title,
		Content:   note.Content,
		CreatedAt: timestamppb.New(note.CreatedAt),
		UpdatedAt: timestamppb.New(note.UpdatedAt),
	})
}

func withRequestID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID := logger.RequestIDFromContext(ctx); requestID != "" {
		return ctx
	}
	if requestID := extractRequestIDFromMetadata(ctx); requestID != "" {
		return logger.NewRequestIDContext(ctx, requestID)
	}
	return logger.NewRequestIDContext(ctx, logger.NewRequestID())
}

func extractRequestIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, header := range []string{"x-request-id", "request-id"} {
		if values := md.Get(header); len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}
