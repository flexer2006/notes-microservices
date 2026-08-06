package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

var (
	errMetadataNotFound       = errors.New("metadata not found in context")
	errAuthHeaderNotFound     = errors.New("authorization header not found")
	errBackendStatus          = errors.New("backend error")
	errStatusMessageUnmatched = errors.New("unmatched status message")
)

//nolint:gochecknoglobals
var statusMessageErrors = map[string]error{
	domain.ErrInvalidCredentials.Error():     domain.ErrInvalidCredentials,
	domain.ErrInvalidRefreshToken.Error():    domain.ErrInvalidRefreshToken,
	domain.ErrRevokedRefreshToken.Error():    domain.ErrRevokedRefreshToken,
	domain.ErrEmailAlreadyExists.Error():     domain.ErrEmailAlreadyExists,
	domain.ErrUserAlreadyExists.Error():      domain.ErrUserAlreadyExists,
	domain.ErrUserNotFound.Error():           domain.ErrUserNotFound,
	domain.ErrNoteNotFound.Error():           domain.ErrNoteNotFound,
	domain.ErrNoteNotFoundOrNotOwned.Error(): domain.ErrNoteNotFoundOrNotOwned,
	domain.ErrUnauthorized.Error():           domain.ErrUnauthorized,
	domain.ErrInvalidEmail.Error():           domain.ErrInvalidEmail,
	domain.ErrEmptyUsername.Error():          domain.ErrEmptyUsername,
	domain.ErrPasswordTooShort.Error():       domain.ErrPasswordTooShort,
	domain.ErrPasswordTooWeak.Error():        domain.ErrPasswordTooWeak,
	domain.ErrEmptyNoteTitle.Error():         domain.ErrEmptyNoteTitle,
	domain.ErrNoteContentTooLarge.Error():    domain.ErrNoteContentTooLarge,
	domain.ErrNoteTitleTooLong.Error():       domain.ErrNoteTitleTooLong,
	domain.ErrUsernameTooLong.Error():        domain.ErrUsernameTooLong,
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errMetadataNotFound
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errAuthHeaderNotFound
	}

	scheme, token, cutOK := strings.Cut(values[0], " ")
	if !cutOK || !strings.EqualFold(scheme, "Bearer") {
		return "", errAuthHeaderNotFound
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", errAuthHeaderNotFound
	}

	return token, nil
}

func outgoingContextWithToken(ctx context.Context, token string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+token)
	if requestID := logger.RequestIDFromContext(ctx); requestID != "" {
		md.Set("x-request-id", requestID)
	}

	return metadata.NewOutgoingContext(ctx, md)
}

func notesStatusFromError(err error) error {
	switch {
	case errors.Is(err, domain.ErrUnauthorized),
		errors.Is(err, domain.ErrInvalidJWTToken),
		errors.Is(err, domain.ErrExpiredJWTToken):
		return status.Error(codes.Unauthenticated, domain.ErrUnauthorized.Error())
	case errors.Is(err, domain.ErrNoteNotFound):
		return status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
	case errors.Is(err, domain.ErrNoteNotFoundOrNotOwned):
		return status.Error(codes.NotFound, domain.ErrNoteNotFoundOrNotOwned.Error())
	case errors.Is(err, domain.ErrEmptyNoteTitle),
		errors.Is(err, domain.ErrNoteTitleTooLong),
		errors.Is(err, domain.ErrNoteContentTooLarge),
		errors.Is(err, domain.ErrEmptyUserID),
		errors.Is(err, domain.ErrInvalidParams):
		switch {
		case errors.Is(err, domain.ErrEmptyNoteTitle):
			return status.Error(codes.InvalidArgument, domain.ErrEmptyNoteTitle.Error())
		case errors.Is(err, domain.ErrNoteTitleTooLong):
			return status.Error(codes.InvalidArgument, domain.ErrNoteTitleTooLong.Error())
		case errors.Is(err, domain.ErrNoteContentTooLarge):
			return status.Error(codes.InvalidArgument, domain.ErrNoteContentTooLarge.Error())
		case errors.Is(err, domain.ErrEmptyUserID):
			return status.Error(codes.InvalidArgument, domain.ErrEmptyUserID.Error())
		default:
			return status.Error(codes.InvalidArgument, domain.ErrInvalidParams.Error())
		}
	default:
		return status.Error(codes.Internal, "internal service error")
	}
}

func authStatusFromError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, domain.ErrInvalidCredentials.Error())
	case errors.Is(err, domain.ErrInvalidRefreshToken):
		return status.Error(codes.Unauthenticated, domain.ErrInvalidRefreshToken.Error())
	case errors.Is(err, domain.ErrRevokedRefreshToken):
		return status.Error(codes.Unauthenticated, domain.ErrRevokedRefreshToken.Error())
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, domain.ErrEmailAlreadyExists.Error())
	case errors.Is(err, domain.ErrUserAlreadyExists):
		return status.Error(codes.AlreadyExists, domain.ErrUserAlreadyExists.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, domain.ErrUserNotFound.Error())
	case errors.Is(err, domain.ErrInvalidEmail):
		return status.Error(codes.InvalidArgument, domain.ErrInvalidEmail.Error())
	case errors.Is(err, domain.ErrEmptyUsername):
		return status.Error(codes.InvalidArgument, domain.ErrEmptyUsername.Error())
	case errors.Is(err, domain.ErrUsernameTooLong):
		return status.Error(codes.InvalidArgument, domain.ErrUsernameTooLong.Error())
	case errors.Is(err, domain.ErrEmptyUserID):
		return status.Error(codes.InvalidArgument, domain.ErrEmptyUserID.Error())
	case errors.Is(err, domain.ErrPasswordTooShort):
		return status.Error(codes.InvalidArgument, domain.ErrPasswordTooShort.Error())
	case errors.Is(err, domain.ErrPasswordTooWeak):
		return status.Error(codes.InvalidArgument, domain.ErrPasswordTooWeak.Error())
	default:
		return status.Error(codes.Internal, "internal service error")
	}
}

func statusMessageError(message string) (bool, error) {
	mapped, ok := statusMessageErrors[message]
	if !ok {
		return false, errStatusMessageUnmatched
	}

	return true, mapped
}

func statusToDomain(err error, notFound error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("%w: %w", domain.ErrServiceUnavailable, err)
	}

	if found, mapped := statusMessageError(st.Message()); found {
		return mapped
	}

	switch st.Code() {
	case codes.Unavailable:
		return domain.ErrServiceUnavailable
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.Unauthenticated:
		return domain.ErrUnauthorized
	case codes.NotFound:
		return notFound
	case codes.AlreadyExists:
		return domain.ErrUserAlreadyExists
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", domain.ErrInvalidParams, st.Message())
	default:
		return fmt.Errorf("%w: %s", errBackendStatus, st.Code())
	}
}

func newClientConn(
	ctx context.Context,
	target string,
	opts ...grpc.DialOption,
) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %q: %w", target, err)
	}

	conn.Connect()

	readyErr := waitUntilReady(ctx, conn)
	if readyErr != nil {
		_ = conn.Close()

		return nil, readyErr
	}

	return conn, nil
}

func waitUntilReady(ctx context.Context, conn *grpc.ClientConn) error {
	for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("%w: %w", domain.ErrServiceUnavailable, ctx.Err())
		}
	}

	return nil
}

func unaryClientTimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if timeout <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		if deadline, ok := ctx.Deadline(); ok {
			if remaining := time.Until(deadline); remaining > 0 && remaining <= timeout {
				return invoker(ctx, method, req, reply, cc, opts...)
			}
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func clientDialOptions(cfg *config.Config) []grpc.DialOption {
	opts := withClientTransport(cfg)

	requestTimeout := time.Duration(0)
	if cfg.GRPCClient != nil {
		requestTimeout = cfg.GRPCClient.RequestTimeout
	}

	if requestTimeout > 0 {
		timeoutInterceptor := unaryClientTimeoutInterceptor(requestTimeout)
		opts = append(opts, grpc.WithUnaryInterceptor(timeoutInterceptor))
	}

	return opts
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

func noteFromProto(protoNote *notesv1.Note) *domain.Note {
	if protoNote == nil {
		return nil
	}

	return new(domain.Note{
		ID:        protoNote.GetNoteId(),
		UserID:    protoNote.GetUserId(),
		Title:     protoNote.GetTitle(),
		Content:   protoNote.GetContent(),
		CreatedAt: protoNote.GetCreatedAt().AsTime(),
		UpdatedAt: protoNote.GetUpdatedAt().AsTime(),
	})
}

func tokenPairFromAuth(
	userID, username, accessToken, refreshToken string,
	expiresAt time.Time,
) *domain.TokenPair {
	return new(domain.TokenPair{
		ExpiresAt:    expiresAt,
		UserID:       userID,
		Username:     username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
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
