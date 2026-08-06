package grpc

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

var (
	errOther = errors.New("other")
	errPlain = errors.New("plain")
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		md      metadata.MD
		want    string
		wantErr error
	}

	cases := []tc{
		{name: "no_md", wantErr: errMetadataNotFound},
		{
			name:    "missing_auth",
			md:      metadata.Pairs("x-request-id", "1"),
			wantErr: errAuthHeaderNotFound,
		},
		{
			name:    "bad_scheme",
			md:      metadata.Pairs("authorization", "Basic x"),
			wantErr: errAuthHeaderNotFound,
		},
		{
			name:    "empty_token",
			md:      metadata.Pairs("authorization", "Bearer "),
			wantErr: errAuthHeaderNotFound,
		},
		{
			name: "ok",
			md:   metadata.Pairs("authorization", "Bearer tok"),
			want: "tok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tc.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tc.md)
			}

			got, err := extractBearerToken(ctx)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil || got != tc.want {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestOutgoingContextWithToken(t *testing.T) {
	t.Parallel()

	ctx := logger.NewRequestIDContext(context.Background(), "rid-1")
	out := outgoingContextWithToken(ctx, "abc")

	md, ok := metadata.FromOutgoingContext(out)
	if !ok {
		t.Fatal("no outgoing md")
	}

	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer abc" {
		t.Fatalf("auth = %v", got)
	}

	if got := md.Get("x-request-id"); len(got) != 1 || got[0] != "rid-1" {
		t.Fatalf("rid = %v", got)
	}
}

func TestNotesStatusFromError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		code codes.Code
		msg  string
	}{
		{
			err: domain.ErrUnauthorized, code: codes.Unauthenticated,
			msg: domain.ErrUnauthorized.Error(),
		},
		{
			err: domain.ErrInvalidJWTToken, code: codes.Unauthenticated,
			msg: domain.ErrUnauthorized.Error(),
		},
		{
			err: domain.ErrExpiredJWTToken, code: codes.Unauthenticated,
			msg: domain.ErrUnauthorized.Error(),
		},
		{err: domain.ErrNoteNotFound, code: codes.NotFound, msg: domain.ErrNoteNotFound.Error()},
		{
			err:  domain.ErrNoteNotFoundOrNotOwned,
			code: codes.NotFound,
			msg:  domain.ErrNoteNotFoundOrNotOwned.Error(),
		},
		{
			err: domain.ErrEmptyNoteTitle, code: codes.InvalidArgument,
			msg: domain.ErrEmptyNoteTitle.Error(),
		},
		{
			err: domain.ErrNoteTitleTooLong, code: codes.InvalidArgument,
			msg: domain.ErrNoteTitleTooLong.Error(),
		},
		{
			err: domain.ErrNoteContentTooLarge, code: codes.InvalidArgument,
			msg: domain.ErrNoteContentTooLarge.Error(),
		},
		{
			err: domain.ErrEmptyUserID, code: codes.InvalidArgument,
			msg: domain.ErrEmptyUserID.Error(),
		},
		{
			err: domain.ErrInvalidParams, code: codes.InvalidArgument,
			msg: domain.ErrInvalidParams.Error(),
		},
		{err: errOther, code: codes.Internal, msg: "internal service error"},
	}

	for _, tc := range cases {
		st, ok := status.FromError(notesStatusFromError(tc.err))
		if !ok || st.Code() != tc.code || st.Message() != tc.msg {
			t.Fatalf("%v -> %v %q", tc.err, st.Code(), st.Message())
		}
	}
}

func TestAuthStatusFromError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		code codes.Code
	}{
		{err: domain.ErrInvalidCredentials, code: codes.Unauthenticated},
		{err: domain.ErrInvalidRefreshToken, code: codes.Unauthenticated},
		{err: domain.ErrRevokedRefreshToken, code: codes.Unauthenticated},
		{err: domain.ErrEmailAlreadyExists, code: codes.AlreadyExists},
		{err: domain.ErrUserAlreadyExists, code: codes.AlreadyExists},
		{err: domain.ErrUserNotFound, code: codes.NotFound},
		{err: domain.ErrInvalidEmail, code: codes.InvalidArgument},
		{err: domain.ErrEmptyUsername, code: codes.InvalidArgument},
		{err: domain.ErrUsernameTooLong, code: codes.InvalidArgument},
		{err: domain.ErrEmptyUserID, code: codes.InvalidArgument},
		{err: domain.ErrPasswordTooShort, code: codes.InvalidArgument},
		{err: domain.ErrPasswordTooWeak, code: codes.InvalidArgument},
		{err: errOther, code: codes.Internal},
	}

	for _, tc := range cases {
		st, ok := status.FromError(authStatusFromError(tc.err))
		if !ok || st.Code() != tc.code {
			t.Fatalf("%v -> %v", tc.err, st.Code())
		}
	}
}

func TestStatusToDomain(t *testing.T) {
	t.Parallel()

	err := statusToDomain(nil, domain.ErrNoteNotFound)
	if err != nil {
		t.Fatalf("nil: %v", err)
	}

	err = statusToDomain(errPlain, domain.ErrNoteNotFound)
	if !errors.Is(err, domain.ErrServiceUnavailable) {
		t.Fatalf("plain: %v", err)
	}

	mapped := status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())

	err = statusToDomain(mapped, domain.ErrUserNotFound)
	if !errors.Is(err, domain.ErrNoteNotFound) {
		t.Fatalf("mapped msg: %v", err)
	}

	cases := []struct {
		code codes.Code
		msg  string
		want error
	}{
		{code: codes.Unavailable, want: domain.ErrServiceUnavailable},
		{code: codes.DeadlineExceeded, want: context.DeadlineExceeded},
		{code: codes.Unauthenticated, want: domain.ErrUnauthorized},
		{code: codes.NotFound, want: domain.ErrNoteNotFound},
		{code: codes.AlreadyExists, want: domain.ErrUserAlreadyExists},
		{
			code: codes.InvalidArgument,
			msg:  domain.ErrInvalidEmail.Error(),
			want: domain.ErrInvalidEmail,
		},
		{code: codes.InvalidArgument, msg: "weird", want: domain.ErrInvalidParams},
		{code: codes.Internal, msg: "x", want: errBackendStatus},
	}

	for _, tc := range cases {
		got := statusToDomain(status.Error(tc.code, tc.msg), domain.ErrNoteNotFound)
		if !errors.Is(got, tc.want) {
			t.Fatalf("%v %q -> %v, want %v", tc.code, tc.msg, got, tc.want)
		}
	}
}

func TestNoteProtoRoundTrip(t *testing.T) {
	t.Parallel()

	if noteToProto(nil) != nil || noteFromProto(nil) != nil {
		t.Fatal("nil mapping")
	}

	now := time.Now().UTC().Truncate(time.Second)
	note := new(domain.Note{
		ID: "n1", UserID: "u1", Title: "t", Content: "c",
		CreatedAt: now, UpdatedAt: now,
	})
	back := noteFromProto(noteToProto(note))

	if back.ID != note.ID || back.Title != note.Title || !back.CreatedAt.Equal(now) {
		t.Fatalf("back = %+v", back)
	}

	_ = noteFromProto(new(notesv1.Note{
		NoteId:    "n2",
		CreatedAt: timestamppb.New(now),
		UpdatedAt: timestamppb.New(now),
	}))
}

func TestTokenPairFromAuth(t *testing.T) {
	t.Parallel()

	exp := time.Now().UTC()

	pair := tokenPairFromAuth("u1", "alice", "a", "r", exp)
	if pair.UserID != "u1" || pair.AccessToken != "a" || !pair.ExpiresAt.Equal(exp) {
		t.Fatalf("%+v", pair)
	}
}

func TestWithRequestID(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context

	ctx := withRequestID(nilCtx)
	if logger.RequestIDFromContext(ctx) == "" {
		t.Fatal("expected generated id")
	}

	keep := logger.NewRequestIDContext(context.Background(), "keep")
	if logger.RequestIDFromContext(withRequestID(keep)) != "keep" {
		t.Fatal("should keep existing")
	}

	mdCtx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("x-request-id", " from-md "),
	)
	if got := logger.RequestIDFromContext(withRequestID(mdCtx)); got != "from-md" {
		t.Fatalf("md id = %q", got)
	}

	mdCtx2 := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("request-id", "alt"),
	)
	if got := logger.RequestIDFromContext(withRequestID(mdCtx2)); got != "alt" {
		t.Fatalf("alt id = %q", got)
	}

	emptyHdr := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("x-request-id", "  "),
	)
	if got := extractRequestIDFromMetadata(emptyHdr); got != "" {
		t.Fatalf("whitespace id = %q", got)
	}

	noKey := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("other", "x"),
	)
	if extractRequestIDFromMetadata(noKey) != "" {
		t.Fatal("unrelated md")
	}

	if extractRequestIDFromMetadata(context.Background()) != "" {
		t.Fatal("no md")
	}
}

func TestUnaryClientTimeoutInterceptor(t *testing.T) {
	t.Parallel()

	invoked := 0
	invoker := func(
		context.Context,
		string,
		any,
		any,
		*grpc.ClientConn,
		...grpc.CallOption,
	) error {
		invoked++

		return nil
	}

	err := unaryClientTimeoutInterceptor(0)(
		context.Background(), "/m", nil, nil, nil, invoker,
	)
	if err != nil || invoked != 1 {
		t.Fatalf("timeout0: err=%v n=%d", err, invoked)
	}

	parent, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond)

	err = unaryClientTimeoutInterceptor(time.Second)(parent, "/m", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("expired parent: %v", err)
	}

	err = unaryClientTimeoutInterceptor(50*time.Millisecond)(
		context.Background(), "/m", nil, nil, nil, invoker,
	)
	if err != nil {
		t.Fatalf("apply timeout: %v", err)
	}

	short, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer shortCancel()

	err = unaryClientTimeoutInterceptor(time.Hour)(short, "/m", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("keep shorter deadline: %v", err)
	}
}

func TestClientDialOptions(t *testing.T) {
	t.Parallel()

	opts := clientDialOptions(new(config.Config{}))
	if len(opts) == 0 {
		t.Fatal("expected transport opts")
	}

	opts = clientDialOptions(new(config.Config{
		GRPCClient: new(config.GRPCClientConfig{RequestTimeout: time.Second, Insecure: true}),
	}))
	if len(opts) < 2 {
		t.Fatalf("opts=%d", len(opts))
	}

	opts = clientDialOptions(new(config.Config{
		GRPCClient: new(config.GRPCClientConfig{Insecure: false}),
	}))
	if len(opts) == 0 {
		t.Fatal("expected tls transport opts")
	}
}

func TestClampInt32(t *testing.T) {
	t.Parallel()

	if clampInt32(-1) != 0 {
		t.Fatal("neg")
	}

	if clampInt32(42) != 42 {
		t.Fatal("mid")
	}

	if clampInt32(math.MaxInt32+1) != math.MaxInt32 {
		t.Fatal("max")
	}
}

func TestClampNonNegative32(t *testing.T) {
	t.Parallel()

	if clampNonNegative32(-1) != 0 {
		t.Fatal("neg")
	}

	if clampNonNegative32(3) != 3 {
		t.Fatal("mid")
	}

	const maxInt32 = int(^uint32(0) >> 1)
	if clampNonNegative32(maxInt32+1) != int32(maxInt32) {
		t.Fatal("max")
	}
}

func TestUnaryAndStreamRequestIDInterceptor(t *testing.T) {
	t.Parallel()

	testkit.UseNopLogger()

	_, err := unaryRequestIDInterceptor(
		context.Background(),
		nil,
		new(grpc.UnaryServerInfo{FullMethod: "/x"}),
		func(context.Context, any) (any, error) { return "ok", nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = unaryRequestIDInterceptor(
		context.Background(),
		nil,
		new(grpc.UnaryServerInfo{FullMethod: "/x"}),
		func(context.Context, any) (any, error) { return nil, errOther },
	)
	if !errors.Is(err, errOther) {
		t.Fatalf("%v", err)
	}

	ss := new(stubStream{ctx: context.Background()})
	streamInfo := new(grpc.StreamServerInfo{FullMethod: "/s"})
	okStream := func(any, grpc.ServerStream) error { return nil }

	err = streamRequestIDInterceptor(nil, ss, streamInfo, okStream)
	if err != nil {
		t.Fatal(err)
	}

	failStream := func(any, grpc.ServerStream) error { return errOther }

	err = streamRequestIDInterceptor(nil, ss, streamInfo, failStream)
	if !errors.Is(err, errOther) {
		t.Fatalf("%v", err)
	}

	wrapped := new(serverStreamWithContext{ServerStream: ss, ctx: context.Background()})
	if wrapped.Context() == nil {
		t.Fatal("nil ctx")
	}
}

//nolint:containedctx
type stubStream struct {
	grpc.ServerStream

	ctx context.Context
}

func (s *stubStream) Context() context.Context { return s.ctx }

func TestNewClientConnMissingTransport(t *testing.T) {
	t.Parallel()

	_, err := newClientConn(context.Background(), "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected dial option error")
	}
}
