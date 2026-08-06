//go:build integration

package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	bcryptadapter "github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
	grpcadapter "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	jwtadapter "github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	pgadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/app"
	"github.com/flexer2006/notes-microservices/internal/testkit"
	"github.com/flexer2006/notes-microservices/internal/testkit/integration"
)

const (
	integrationBufSize       = 1024 * 1024
	integrationAccessSecret  = "nm-integration-access-secret-key"
	integrationRefreshSecret = "nm-integration-refresh-secret-key"
	integrationPassword      = "password1"
)

func startIntegrationBuf(
	t *testing.T,
	interceptor grpc.UnaryServerInterceptor,
	register func(grpc.ServiceRegistrar),
) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(integrationBufSize)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor))
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

func withBearer(ctx context.Context, token string) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
}

func TestAuthNotesGRPCIntegration(t *testing.T) {
	testkit.UseNopLogger()

	accessTTL := 15 * time.Minute
	refreshTTL := 24 * time.Hour
	issuer := jwtadapter.NewIssuer(
		integrationAccessSecret,
		integrationRefreshSecret,
		accessTTL,
		refreshTTL,
	)
	verifier := jwtadapter.NewAccessVerifier(integrationAccessSecret)

	authDB := integration.StartPostgres(t, "auth")
	authFactory := pgadapter.NewAuthRepositoryFactory(authDB.Pool())
	authUC := app.NewAuthUseCase(
		authFactory.UserRepository(),
		authFactory.TokenRepository(),
		bcryptadapter.NewBcrypt(bcrypt.MinCost),
		issuer,
	)
	userUC := app.NewUserUseCase(authFactory.UserRepository())

	authConn := startIntegrationBuf(
		t,
		grpcadapter.NewUnaryAuthInterceptor(
			issuer,
			authv1.UserService_GetUserProfile_FullMethodName,
		),
		func(reg grpc.ServiceRegistrar) {
			grpcadapter.NewAuthHandler(authUC).RegisterService(reg)
			grpcadapter.NewUserHandler(userUC).RegisterService(reg)
		},
	)

	notesDB := integration.StartPostgres(t, "notes")
	noteUC := app.NewNoteUseCase(pgadapter.NewNoteRepository(notesDB.Pool()))

	notesConn := startIntegrationBuf(
		t,
		grpcadapter.NewUnaryAuthInterceptor(
			verifier,
			notesv1.NoteService_CreateNote_FullMethodName,
			notesv1.NoteService_GetNote_FullMethodName,
			notesv1.NoteService_ListNotes_FullMethodName,
			notesv1.NoteService_UpdateNote_FullMethodName,
			notesv1.NoteService_DeleteNote_FullMethodName,
		),
		func(reg grpc.ServiceRegistrar) {
			grpcadapter.NewNoteHandler(noteUC).RegisterService(reg)
		},
	)

	authClient := authv1.NewAuthServiceClient(authConn)
	userClient := authv1.NewUserServiceClient(authConn)
	notesClient := notesv1.NewNoteServiceClient(notesConn)
	ctx := context.Background()

	_, err := notesClient.CreateNote(ctx, new(notesv1.CreateNoteRequest{Title: "t", Content: "c"}))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauth create: %v", err)
	}

	reg, err := authClient.Register(ctx, new(authv1.RegisterRequest{
		Email: "alice@example.com", Username: "alice", Password: integrationPassword,
	}))
	testkit.MyErrIs(t, err, nil)

	if reg.GetAccessToken() == "" || reg.GetUserId() == "" {
		t.Fatalf("%+v", reg)
	}

	login, err := authClient.Login(ctx, new(authv1.LoginRequest{
		Email: "alice@example.com", Password: integrationPassword,
	}))
	testkit.MyErrIs(t, err, nil)

	access := login.GetAccessToken()
	if access == "" {
		t.Fatal("empty access token")
	}

	profile, err := userClient.GetUserProfile(withBearer(ctx, access), new(emptypb.Empty{}))
	testkit.MyErrIs(t, err, nil)

	if profile.GetUsername() != "alice" {
		t.Fatalf("%+v", profile)
	}

	created, err := notesClient.CreateNote(
		withBearer(ctx, access),
		new(notesv1.CreateNoteRequest{Title: "first", Content: "body"}),
	)
	testkit.MyErrIs(t, err, nil)

	noteID := created.GetNote().GetNoteId()
	if noteID == "" || created.GetNote().GetUserId() != reg.GetUserId() {
		t.Fatalf("%+v", created)
	}

	got, err := notesClient.GetNote(
		withBearer(ctx, access),
		new(notesv1.GetNoteRequest{NoteId: noteID}),
	)
	testkit.MyErrIs(t, err, nil)

	if got.GetNote().GetTitle() != "first" {
		t.Fatalf("%+v", got)
	}

	title := "updated"
	updated, err := notesClient.UpdateNote(
		withBearer(ctx, access),
		new(notesv1.UpdateNoteRequest{NoteId: noteID, Title: new(title)}),
	)
	testkit.MyErrIs(t, err, nil)

	if updated.GetNote().GetTitle() != title {
		t.Fatalf("%+v", updated)
	}

	list, err := notesClient.ListNotes(
		withBearer(ctx, access),
		new(notesv1.ListNotesRequest{Limit: 10, Offset: 0}),
	)
	testkit.MyErrIs(t, err, nil)

	if list.GetTotalCount() != 1 || len(list.GetNotes()) != 1 {
		t.Fatalf("%+v", list)
	}

	_, err = notesClient.DeleteNote(
		withBearer(ctx, access),
		new(notesv1.DeleteNoteRequest{NoteId: noteID}),
	)
	testkit.MyErrIs(t, err, nil)

	_, err = notesClient.GetNote(
		withBearer(ctx, access),
		new(notesv1.GetNoteRequest{NoteId: noteID}),
	)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("deleted get: %v", err)
	}

	_, err = authClient.Logout(
		ctx,
		new(authv1.LogoutRequest{RefreshToken: login.GetRefreshToken()}),
	)
	testkit.MyErrIs(t, err, nil)
}
