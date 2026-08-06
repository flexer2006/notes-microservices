package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	grpcadapter "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

var errBoom = errors.New("boom")

func pair() *domain.TokenPair {
	return new(domain.TokenPair{
		UserID: "u1", Username: "alice", AccessToken: "a", RefreshToken: "r",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
}

func note() *domain.Note {
	return new(domain.Note{ID: "n1", UserID: "u1", Title: "t", Content: "c"})
}

type stubAuthUC struct {
	register func(context.Context, string, string, string) (*domain.TokenPair, error)
	login    func(context.Context, string, string) (*domain.TokenPair, error)
	refresh  func(context.Context, string) (*domain.TokenPair, error)
	logout   func(context.Context, string) error
}

func (s *stubAuthUC) Register(
	ctx context.Context,
	email, user, pass string,
) (*domain.TokenPair, error) {
	return s.register(ctx, email, user, pass)
}

func (s *stubAuthUC) Login(ctx context.Context, email, pass string) (*domain.TokenPair, error) {
	return s.login(ctx, email, pass)
}

func (s *stubAuthUC) RefreshTokens(ctx context.Context, tok string) (*domain.TokenPair, error) {
	return s.refresh(ctx, tok)
}

func (s *stubAuthUC) Logout(ctx context.Context, tok string) error { return s.logout(ctx, tok) }

type stubUserUC struct {
	get func(context.Context, string) (*domain.User, error)
}

func (s *stubUserUC) GetUserProfile(ctx context.Context, id string) (*domain.User, error) {
	return s.get(ctx, id)
}

type stubNoteUC struct {
	create func(context.Context, string, string, string) (*domain.Note, error)
	get    func(context.Context, string, string) (*domain.Note, error)
	list   func(context.Context, string, int, int) ([]*domain.Note, int, error)
	update func(context.Context, string, string, *string, *string) (*domain.Note, error)
	delete func(context.Context, string, string) error
}

func (s *stubNoteUC) CreateNote(
	ctx context.Context,
	uid, title, content string,
) (*domain.Note, error) {
	return s.create(ctx, uid, title, content)
}

func (s *stubNoteUC) GetNote(ctx context.Context, uid, id string) (*domain.Note, error) {
	return s.get(ctx, uid, id)
}

func (s *stubNoteUC) ListNotes(
	ctx context.Context,
	uid string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	return s.list(ctx, uid, limit, offset)
}

func (s *stubNoteUC) UpdateNote(
	ctx context.Context,
	uid, id string,
	title, content *string,
) (*domain.Note, error) {
	return s.update(ctx, uid, id, title, content)
}

func (s *stubNoteUC) DeleteNote(ctx context.Context, uid, id string) error {
	return s.delete(ctx, uid, id)
}

type stubTokenSvc struct {
	validate func(context.Context, string) (string, error)
}

func (s *stubTokenSvc) GenerateAccessToken(
	context.Context,
	string,
	string,
) (string, time.Time, error) {
	panic("unexpected")
}

func (s *stubTokenSvc) GenerateRefreshToken(context.Context, string) (string, time.Time, error) {
	panic("unexpected")
}

func (s *stubTokenSvc) ValidateAccessToken(ctx context.Context, tok string) (string, error) {
	return s.validate(ctx, tok)
}

func withUser(ctx context.Context) context.Context {
	return authctx.WithUserID(ctx, "u1")
}

func requireStatus(t *testing.T, err error, want codes.Code) {
	t.Helper()

	if got := status.Code(err); got != want {
		t.Fatalf("status=%v want=%v err=%v", got, want, err)
	}
}

func TestAuthHandler(t *testing.T) {
	t.Parallel()

	uc := new(stubAuthUC{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		logout: func(context.Context, string) error { return nil },
	})
	h := grpcadapter.NewAuthHandler(uc)

	_, err := h.Register(context.Background(), new(authv1.RegisterRequest{}))
	requireStatus(t, err, codes.InvalidArgument)

	resp, err := h.Register(context.Background(), new(authv1.RegisterRequest{
		Email: "a@ex.com", Username: "alice", Password: "password1",
	}))
	testkit.MyErrIs(t, err, nil)

	if resp.GetUserId() != "u1" {
		t.Fatalf("%+v", resp)
	}

	uc.register = func(context.Context, string, string, string) (*domain.TokenPair, error) {
		return nil, domain.ErrEmailAlreadyExists
	}

	_, err = h.Register(context.Background(), new(authv1.RegisterRequest{
		Email: "a@ex.com", Username: "alice", Password: "password1",
	}))
	requireStatus(t, err, codes.AlreadyExists)

	_, err = h.Login(context.Background(), new(authv1.LoginRequest{}))
	requireStatus(t, err, codes.InvalidArgument)

	_, err = h.Login(context.Background(), new(authv1.LoginRequest{
		Email: "a@ex.com", Password: "password1",
	}))
	testkit.MyErrIs(t, err, nil)

	uc.login = func(context.Context, string, string) (*domain.TokenPair, error) {
		return nil, domain.ErrInvalidCredentials
	}

	_, err = h.Login(context.Background(), new(authv1.LoginRequest{
		Email: "a@ex.com", Password: "password1",
	}))
	requireStatus(t, err, codes.Unauthenticated)

	_, err = h.RefreshTokens(context.Background(), new(authv1.RefreshTokensRequest{}))
	requireStatus(t, err, codes.InvalidArgument)

	_, err = h.RefreshTokens(
		context.Background(),
		new(authv1.RefreshTokensRequest{RefreshToken: "r"}),
	)
	testkit.MyErrIs(t, err, nil)

	uc.refresh = func(context.Context, string) (*domain.TokenPair, error) {
		return nil, domain.ErrRevokedRefreshToken
	}

	_, err = h.RefreshTokens(
		context.Background(),
		new(authv1.RefreshTokensRequest{RefreshToken: "r"}),
	)
	requireStatus(t, err, codes.Unauthenticated)

	_, err = h.Logout(context.Background(), new(authv1.LogoutRequest{}))
	requireStatus(t, err, codes.InvalidArgument)

	_, err = h.Logout(context.Background(), new(authv1.LogoutRequest{RefreshToken: "r"}))
	testkit.MyErrIs(t, err, nil)

	uc.logout = func(context.Context, string) error { return errBoom }

	_, err = h.Logout(context.Background(), new(authv1.LogoutRequest{RefreshToken: "r"}))
	requireStatus(t, err, codes.Internal)

	srv := grpc.NewServer()
	h.RegisterService(srv)
}

func TestUserHandler(t *testing.T) {
	t.Parallel()

	uc := new(stubUserUC{
		get: func(context.Context, string) (*domain.User, error) {
			return new(domain.User{ID: "u1", Email: "a@ex.com", Username: "alice"}), nil
		},
	})
	h := grpcadapter.NewUserHandler(uc)

	_, err := h.GetUserProfile(context.Background(), new(emptypb.Empty{}))
	requireStatus(t, err, codes.Unauthenticated)

	resp, err := h.GetUserProfile(withUser(context.Background()), new(emptypb.Empty{}))
	testkit.MyErrIs(t, err, nil)

	if resp.GetUserId() != "u1" {
		t.Fatalf("%+v", resp)
	}

	uc.get = func(context.Context, string) (*domain.User, error) {
		return nil, domain.ErrUserNotFound
	}

	_, err = h.GetUserProfile(withUser(context.Background()), new(emptypb.Empty{}))
	requireStatus(t, err, codes.NotFound)

	srv := grpc.NewServer()
	h.RegisterService(srv)
}

func TestNoteHandler(t *testing.T) {
	t.Parallel()

	uc := new(stubNoteUC{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return note(), nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) { return note(), nil },
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return []*domain.Note{note()}, 1, nil
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return note(), nil
		},
		delete: func(context.Context, string, string) error { return nil },
	})
	h := grpcadapter.NewNoteHandler(uc)
	ctx := withUser(context.Background())
	bare := context.Background()

	for _, call := range []func() error{
		func() error {
			_, e := h.CreateNote(bare, new(notesv1.CreateNoteRequest{Title: "t"}))

			return e
		},
		func() error {
			_, e := h.GetNote(bare, new(notesv1.GetNoteRequest{NoteId: "n1"}))

			return e
		},
		func() error {
			_, e := h.ListNotes(bare, new(notesv1.ListNotesRequest{}))

			return e
		},
		func() error {
			_, e := h.UpdateNote(bare, new(notesv1.UpdateNoteRequest{NoteId: "n1"}))

			return e
		},
		func() error {
			_, e := h.DeleteNote(bare, new(notesv1.DeleteNoteRequest{NoteId: "n1"}))

			return e
		},
	} {
		requireStatus(t, call(), codes.Unauthenticated)
	}

	resp, err := h.CreateNote(ctx, new(notesv1.CreateNoteRequest{Title: "t", Content: "c"}))
	testkit.MyErrIs(t, err, nil)

	if resp.GetNote().GetNoteId() != "n1" {
		t.Fatalf("%+v", resp)
	}

	uc.create = func(context.Context, string, string, string) (*domain.Note, error) {
		return nil, domain.ErrEmptyNoteTitle
	}

	_, err = h.CreateNote(ctx, new(notesv1.CreateNoteRequest{Title: "t"}))
	requireStatus(t, err, codes.InvalidArgument)

	_, err = h.GetNote(ctx, new(notesv1.GetNoteRequest{NoteId: "n1"}))
	testkit.MyErrIs(t, err, nil)

	uc.get = func(context.Context, string, string) (*domain.Note, error) {
		return nil, domain.ErrNoteNotFound
	}

	_, err = h.GetNote(ctx, new(notesv1.GetNoteRequest{NoteId: "n1"}))
	requireStatus(t, err, codes.NotFound)

	list, err := h.ListNotes(ctx, new(notesv1.ListNotesRequest{Limit: 10, Offset: 0}))
	testkit.MyErrIs(t, err, nil)

	if list.GetTotalCount() != 1 {
		t.Fatalf("%+v", list)
	}

	uc.list = func(context.Context, string, int, int) ([]*domain.Note, int, error) {
		return nil, 0, errBoom
	}

	_, err = h.ListNotes(ctx, new(notesv1.ListNotesRequest{}))
	requireStatus(t, err, codes.Internal)

	title := "x"
	_, err = h.UpdateNote(ctx, new(notesv1.UpdateNoteRequest{NoteId: "n1", Title: new(title)}))
	testkit.MyErrIs(t, err, nil)

	uc.update = func(context.Context, string, string, *string, *string) (*domain.Note, error) {
		return nil, domain.ErrNoteNotFoundOrNotOwned
	}

	_, err = h.UpdateNote(ctx, new(notesv1.UpdateNoteRequest{NoteId: "n1"}))
	requireStatus(t, err, codes.NotFound)

	_, err = h.DeleteNote(ctx, new(notesv1.DeleteNoteRequest{NoteId: "n1"}))
	testkit.MyErrIs(t, err, nil)

	uc.delete = func(context.Context, string, string) error { return errBoom }

	_, err = h.DeleteNote(ctx, new(notesv1.DeleteNoteRequest{NoteId: "n1"}))
	requireStatus(t, err, codes.Internal)

	srv := grpc.NewServer()
	h.RegisterService(srv)
}

func TestUnaryAuthInterceptor(t *testing.T) {
	t.Parallel()

	tok := new(stubTokenSvc{
		validate: func(context.Context, string) (string, error) { return "u1", nil },
	})
	interceptor := grpcadapter.NewUnaryAuthInterceptor(tok, "/pkg.Service/Protected")

	handler := func(context.Context, any) (any, error) {
		return "ok", nil
	}

	info := new(grpc.UnaryServerInfo{FullMethod: "/pkg.Service/Open"})
	out, err := interceptor(context.Background(), nil, info, handler)
	testkit.MyErrIs(t, err, nil)

	if out != "ok" {
		t.Fatalf("%v", out)
	}

	protected := func(ctx context.Context, _ any) (any, error) {
		if authctx.UserIDFrom(ctx) != "u1" {
			return nil, errBoom
		}

		return "ok", nil
	}

	info.FullMethod = "/pkg.Service/Protected"

	_, err = interceptor(context.Background(), nil, info, protected)
	requireStatus(t, err, codes.Unauthenticated)

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", "Bearer good"),
	)
	out, err = interceptor(ctx, nil, info, protected)
	testkit.MyErrIs(t, err, nil)

	if out != "ok" {
		t.Fatalf("%v", out)
	}

	tok.validate = func(context.Context, string) (string, error) {
		return "", domain.ErrInvalidJWTToken
	}

	_, err = interceptor(ctx, nil, info, protected)
	requireStatus(t, err, codes.Unauthenticated)
}

func TestServerLifecycle(t *testing.T) {
	t.Parallel()

	cfg := new(config.Config{
		GRPC: new(config.GRPCConfig{Host: "127.0.0.1", Port: 0, Reflection: true}),
	})
	srv := grpcadapter.New(cfg)
	srv.RegisterService(func(reg grpc.ServiceRegistrar) {
		grpcadapter.NewNoteHandler(new(stubNoteUC{
			create: func(context.Context, string, string, string) (*domain.Note, error) {
				return note(), nil
			},
			get: func(context.Context, string, string) (*domain.Note, error) { return note(), nil },
			list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
				return nil, 0, nil
			},
			update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
				return note(), nil
			},
			delete: func(context.Context, string, string) error { return nil },
		})).RegisterService(reg)
	})

	ctx := context.Background()
	err := srv.Start(ctx)
	testkit.MyErrIs(t, err, nil)

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	srv.Stop(stopCtx)
	_ = srv.Err()

	prod := grpcadapter.New(new(config.Config{
		GRPC:    new(config.GRPCConfig{Host: "127.0.0.1", Port: 0}),
		Logging: new(config.LoggingConfig{Mode: "production"}),
	}))
	testkit.MyErrIs(t, prod.Start(context.Background()), nil)

	forceStop, forceCancel := context.WithCancel(context.Background())
	forceCancel()
	prod.Stop(forceStop)

	noLog := grpcadapter.New(new(config.Config{
		GRPC: new(config.GRPCConfig{Host: "127.0.0.1", Port: 0}),
	}))
	testkit.MyErrIs(t, noLog.Start(context.Background()), nil)
	noLog.Stop(context.Background())

	_ = grpcadapter.New(nil)

	held, holdErr := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	testkit.MyErrIs(t, holdErr, nil)

	t.Cleanup(func() {
		closeErr := held.Close()
		if closeErr != nil {
			t.Errorf("close listener: %v", closeErr)
		}
	})

	tcpAddr, ok := held.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type %T", held.Addr())
	}

	busy := grpcadapter.New(new(config.Config{
		GRPC: new(config.GRPCConfig{Host: "127.0.0.1", Port: tcpAddr.Port}),
	}))

	startErr := busy.Start(context.Background())
	if startErr == nil {
		t.Fatal("expected listen failure")
	}
}
