package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

const bufSize = 1024 * 1024

func startBufServer(t *testing.T, register func(grpc.ServiceRegistrar)) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	register(srv)

	go func() { _ = srv.Serve(lis) }()

	t.Cleanup(func() {
		srv.Stop()

		_ = lis.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///buf",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func TestNotesClientBufConn(t *testing.T) {
	t.Parallel()

	testkit.UseNopLogger()

	noteUC := new(bufNoteUC{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1", UserID: "u1", Title: "t", Content: "c"}), nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1", UserID: "u1", Title: "t"}), nil
		},
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return []*domain.Note{{ID: "n1", UserID: "u1", Title: "t"}}, 1, nil
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1", Title: "x"}), nil
		},
		delete: func(context.Context, string, string) error { return nil },
	})

	conn := startBufServer(t, func(reg grpc.ServiceRegistrar) {
		h := NewNoteHandler(noteUC)
		notesv1.RegisterNoteServiceServer(reg, new(userInjectNote{NoteHandler: h}))
	})

	client := new(NotesClient{
		notesClient: notesv1.NewNoteServiceClient(conn),
		conn:        conn,
	})

	n, err := client.CreateNote(context.Background(), "tok", "t", "c")
	testkit.MyErrIs(t, err, nil)

	if n.ID != "n1" {
		t.Fatalf("%+v", n)
	}

	_, err = client.GetNote(context.Background(), "tok", "n1")
	testkit.MyErrIs(t, err, nil)

	list, total, err := client.ListNotes(context.Background(), "tok", 10, 0)
	testkit.MyErrIs(t, err, nil)

	if total != 1 || len(list) != 1 {
		t.Fatalf("%d %d", total, len(list))
	}

	title := "x"
	_, err = client.UpdateNote(context.Background(), "tok", "n1", new(title), nil)
	testkit.MyErrIs(t, err, nil)

	testkit.MyErrIs(t, client.DeleteNote(context.Background(), "tok", "n1"), nil)
	testkit.MyErrIs(t, client.Close(), nil)

	err = client.Close()
	if err == nil {
		t.Fatal("expected close error after conn closed")
	}

	testkit.MyErrIs(t, (new(NotesClient{})).Close(), nil)

	failConn := startBufServer(t, func(reg grpc.ServiceRegistrar) {
		notesv1.RegisterNoteServiceServer(reg, new(failNoteServer{}))
	})
	failClient := new(NotesClient{
		notesClient: notesv1.NewNoteServiceClient(failConn),
		conn:        failConn,
	})

	_, err = failClient.CreateNote(context.Background(), "tok", "t", "c")
	if err == nil {
		t.Fatal("create err")
	}

	_, err = failClient.GetNote(context.Background(), "tok", "n1")
	if err == nil {
		t.Fatal("get err")
	}

	_, _, err = failClient.ListNotes(context.Background(), "tok", 1, 0)
	if err == nil {
		t.Fatal("list err")
	}

	_, err = failClient.UpdateNote(context.Background(), "tok", "n1", nil, nil)
	if err == nil {
		t.Fatal("update err")
	}

	err = failClient.DeleteNote(context.Background(), "tok", "n1")
	if err == nil {
		t.Fatal("delete err")
	}

	_, err = NewNotesClient(context.Background(), new(config.Config{}))
	if err == nil {
		t.Fatal("expected unavailable")
	}
}

type failNoteServer struct {
	notesv1.UnimplementedNoteServiceServer
}

func (failNoteServer) CreateNote(
	context.Context,
	*notesv1.CreateNoteRequest,
) (*notesv1.NoteResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
}

func (failNoteServer) GetNote(
	context.Context,
	*notesv1.GetNoteRequest,
) (*notesv1.NoteResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
}

func (failNoteServer) ListNotes(
	context.Context,
	*notesv1.ListNotesRequest,
) (*notesv1.ListNotesResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
}

func (failNoteServer) UpdateNote(
	context.Context,
	*notesv1.UpdateNoteRequest,
) (*notesv1.NoteResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
}

func (failNoteServer) DeleteNote(
	context.Context,
	*notesv1.DeleteNoteRequest,
) (*emptypb.Empty, error) {
	return nil, status.Error(codes.NotFound, domain.ErrNoteNotFound.Error())
}

type bufNoteUC struct {
	create func(context.Context, string, string, string) (*domain.Note, error)
	get    func(context.Context, string, string) (*domain.Note, error)
	list   func(context.Context, string, int, int) ([]*domain.Note, int, error)
	update func(context.Context, string, string, *string, *string) (*domain.Note, error)
	delete func(context.Context, string, string) error
}

func (s *bufNoteUC) CreateNote(
	ctx context.Context,
	uid, title, content string,
) (*domain.Note, error) {
	return s.create(ctx, uid, title, content)
}

func (s *bufNoteUC) GetNote(ctx context.Context, uid, id string) (*domain.Note, error) {
	return s.get(ctx, uid, id)
}

func (s *bufNoteUC) ListNotes(
	ctx context.Context,
	uid string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	return s.list(ctx, uid, limit, offset)
}

func (s *bufNoteUC) UpdateNote(
	ctx context.Context,
	uid, id string,
	title, content *string,
) (*domain.Note, error) {
	return s.update(ctx, uid, id, title, content)
}

func (s *bufNoteUC) DeleteNote(ctx context.Context, uid, id string) error {
	return s.delete(ctx, uid, id)
}

type userInjectNote struct {
	*NoteHandler
}

func (h *userInjectNote) CreateNote(
	ctx context.Context,
	req *notesv1.CreateNoteRequest,
) (*notesv1.NoteResponse, error) {
	return h.NoteHandler.CreateNote(authctx.WithUserID(ctx, "u1"), req)
}

func (h *userInjectNote) GetNote(
	ctx context.Context,
	req *notesv1.GetNoteRequest,
) (*notesv1.NoteResponse, error) {
	return h.NoteHandler.GetNote(authctx.WithUserID(ctx, "u1"), req)
}

func (h *userInjectNote) ListNotes(
	ctx context.Context,
	req *notesv1.ListNotesRequest,
) (*notesv1.ListNotesResponse, error) {
	return h.NoteHandler.ListNotes(authctx.WithUserID(ctx, "u1"), req)
}

func (h *userInjectNote) UpdateNote(
	ctx context.Context,
	req *notesv1.UpdateNoteRequest,
) (*notesv1.NoteResponse, error) {
	return h.NoteHandler.UpdateNote(authctx.WithUserID(ctx, "u1"), req)
}

func (h *userInjectNote) DeleteNote(
	ctx context.Context,
	req *notesv1.DeleteNoteRequest,
) (*emptypb.Empty, error) {
	return h.NoteHandler.DeleteNote(authctx.WithUserID(ctx, "u1"), req)
}

func TestAuthClientBufConn(t *testing.T) {
	t.Parallel()

	testkit.UseNopLogger()

	authUC := new(bufAuthUC{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", Username: "alice", AccessToken: "a", RefreshToken: "r",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", AccessToken: "a", RefreshToken: "r",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", AccessToken: "a", RefreshToken: "r2",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		logout: func(context.Context, string) error { return nil },
	})
	userUC := new(bufUserUC{
		get: func(context.Context, string) (*domain.User, error) {
			return new(domain.User{ID: "u1", Email: "a@ex.com", Username: "alice"}), nil
		},
	})

	conn := startBufServer(t, func(reg grpc.ServiceRegistrar) {
		NewAuthHandler(authUC).RegisterService(reg)
		authv1.RegisterUserServiceServer(
			reg,
			new(userInjectUser{UserHandler: NewUserHandler(userUC)}),
		)
	})

	client := new(AuthClient{
		authClient: authv1.NewAuthServiceClient(conn),
		userClient: authv1.NewUserServiceClient(conn),
		conn:       conn,
	})

	_, err := client.Register(context.Background(), "a@ex.com", "alice", "password1")
	testkit.MyErrIs(t, err, nil)

	_, err = client.Login(context.Background(), "a@ex.com", "password1")
	testkit.MyErrIs(t, err, nil)

	_, err = client.RefreshTokens(context.Background(), "r")
	testkit.MyErrIs(t, err, nil)

	testkit.MyErrIs(t, client.Logout(context.Background(), "r"), nil)

	_, err = client.GetUserProfile(context.Background(), "tok")
	testkit.MyErrIs(t, err, nil)

	testkit.MyErrIs(t, client.Close(), nil)

	err = client.Close()
	if err == nil {
		t.Fatal("expected close error after conn closed")
	}

	testkit.MyErrIs(t, (new(AuthClient{})).Close(), nil)

	failConn := startBufServer(t, func(reg grpc.ServiceRegistrar) {
		authv1.RegisterAuthServiceServer(reg, new(failAuthServer{}))
		authv1.RegisterUserServiceServer(reg, new(failUserServer{}))
	})
	failClient := new(AuthClient{
		authClient: authv1.NewAuthServiceClient(failConn),
		userClient: authv1.NewUserServiceClient(failConn),
		conn:       failConn,
	})

	_, err = failClient.Register(context.Background(), "a@ex.com", "alice", "password1")
	if err == nil {
		t.Fatal("register err")
	}

	_, err = failClient.Login(context.Background(), "a@ex.com", "password1")
	if err == nil {
		t.Fatal("login err")
	}

	_, err = failClient.RefreshTokens(context.Background(), "r")
	if err == nil {
		t.Fatal("refresh err")
	}

	err = failClient.Logout(context.Background(), "r")
	if err == nil {
		t.Fatal("logout err")
	}

	_, err = failClient.GetUserProfile(context.Background(), "tok")
	if err == nil {
		t.Fatal("profile err")
	}

	_, err = NewAuthClient(context.Background(), new(config.Config{}))
	if err == nil {
		t.Fatal("expected unavailable")
	}
}

type failAuthServer struct {
	authv1.UnimplementedAuthServiceServer
}

func (failAuthServer) Register(
	context.Context,
	*authv1.RegisterRequest,
) (*authv1.RegisterResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrUserNotFound.Error())
}

func (failAuthServer) Login(
	context.Context,
	*authv1.LoginRequest,
) (*authv1.LoginResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrUserNotFound.Error())
}

func (failAuthServer) RefreshTokens(
	context.Context,
	*authv1.RefreshTokensRequest,
) (*authv1.RefreshTokensResponse, error) {
	return nil, status.Error(codes.Unauthenticated, domain.ErrInvalidRefreshToken.Error())
}

func (failAuthServer) Logout(context.Context, *authv1.LogoutRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.NotFound, domain.ErrUserNotFound.Error())
}

type failUserServer struct {
	authv1.UnimplementedUserServiceServer
}

func (failUserServer) GetUserProfile(
	context.Context,
	*emptypb.Empty,
) (*authv1.UserProfileResponse, error) {
	return nil, status.Error(codes.NotFound, domain.ErrUserNotFound.Error())
}

func TestNewClientsLoopbackDial(t *testing.T) {
	t.Parallel()

	testkit.UseNopLogger()

	noteUC := new(bufNoteUC{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1", UserID: "u1", Title: "t"}), nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1"}), nil
		},
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return nil, 0, nil
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return new(domain.Note{ID: "n1"}), nil
		},
		delete: func(context.Context, string, string) error { return nil },
	})
	authUC := new(bufAuthUC{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", AccessToken: "a", RefreshToken: "r",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", AccessToken: "a", RefreshToken: "r",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return new(domain.TokenPair{
				UserID: "u1", AccessToken: "a", RefreshToken: "r",
				ExpiresAt: time.Now().Add(time.Hour),
			}), nil
		},
		logout: func(context.Context, string) error { return nil },
	})
	userUC := new(bufUserUC{
		get: func(context.Context, string) (*domain.User, error) {
			return new(domain.User{ID: "u1", Email: "a@ex.com", Username: "alice"}), nil
		},
	})

	lis, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := grpc.NewServer()
	notesv1.RegisterNoteServiceServer(srv, new(userInjectNote{NoteHandler: NewNoteHandler(noteUC)}))
	NewAuthHandler(authUC).RegisterService(srv)
	authv1.RegisterUserServiceServer(srv, new(userInjectUser{UserHandler: NewUserHandler(userUC)}))

	go func() { _ = srv.Serve(lis) }()

	t.Cleanup(func() {
		srv.Stop()

		_ = lis.Close()
	})

	port := tcpPort(t, lis.Addr())
	cfg := new(config.Config{
		GRPCClient: new(config.GRPCClientConfig{
			NotesService: config.GRPCServiceConfig{
				Host: "127.0.0.1", Port: port, ConnectTimeout: 2 * time.Second,
			},
			AuthService: config.GRPCServiceConfig{
				Host: "127.0.0.1", Port: port, ConnectTimeout: 2 * time.Second,
			},
			Insecure: true,
		}),
	})

	notesClient, err := NewNotesClient(context.Background(), cfg)
	testkit.MyErrIs(t, err, nil)
	testkit.MyErrIs(t, notesClient.Close(), nil)

	authClient, err := NewAuthClient(context.Background(), cfg)
	testkit.MyErrIs(t, err, nil)
	testkit.MyErrIs(t, authClient.Close(), nil)

	dead := new(config.Config{
		GRPCClient: new(config.GRPCClientConfig{
			NotesService: config.GRPCServiceConfig{
				Host: "127.0.0.1", Port: 1, ConnectTimeout: 50 * time.Millisecond,
			},
			AuthService: config.GRPCServiceConfig{
				Host: "127.0.0.1", Port: 1, ConnectTimeout: 50 * time.Millisecond,
			},
			Insecure: true,
		}),
	})

	_, err = NewNotesClient(context.Background(), dead)
	if err == nil {
		t.Fatal("expected notes dial timeout")
	}

	_, err = NewAuthClient(context.Background(), dead)
	if err == nil {
		t.Fatal("expected auth dial timeout")
	}
}

type bufAuthUC struct {
	register func(context.Context, string, string, string) (*domain.TokenPair, error)
	login    func(context.Context, string, string) (*domain.TokenPair, error)
	refresh  func(context.Context, string) (*domain.TokenPair, error)
	logout   func(context.Context, string) error
}

func (s *bufAuthUC) Register(
	ctx context.Context,
	email, user, pass string,
) (*domain.TokenPair, error) {
	return s.register(ctx, email, user, pass)
}

func (s *bufAuthUC) Login(ctx context.Context, email, pass string) (*domain.TokenPair, error) {
	return s.login(ctx, email, pass)
}

func (s *bufAuthUC) RefreshTokens(ctx context.Context, tok string) (*domain.TokenPair, error) {
	return s.refresh(ctx, tok)
}

func (s *bufAuthUC) Logout(ctx context.Context, tok string) error { return s.logout(ctx, tok) }

type bufUserUC struct {
	get func(context.Context, string) (*domain.User, error)
}

func (s *bufUserUC) GetUserProfile(ctx context.Context, id string) (*domain.User, error) {
	return s.get(ctx, id)
}

type userInjectUser struct {
	*UserHandler
}

func (h *userInjectUser) GetUserProfile(
	ctx context.Context,
	req *emptypb.Empty,
) (*authv1.UserProfileResponse, error) {
	return h.UserHandler.GetUserProfile(authctx.WithUserID(ctx, "u1"), req)
}

func tcpPort(t *testing.T, addr net.Addr) int {
	t.Helper()

	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type %T", addr)
	}

	return tcpAddr.Port
}
