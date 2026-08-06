package grpc

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
)

type NotesClient struct {
	notesClient notesv1.NoteServiceClient
	conn        *grpc.ClientConn
}

type AuthClient struct {
	authClient authv1.AuthServiceClient
	userClient authv1.UserServiceClient
	conn       *grpc.ClientConn
}

func NewNotesClient(ctx context.Context, cfg *config.Config) (*NotesClient, error) {
	if cfg.GRPCClient == nil {
		return nil, fmt.Errorf("notes service: %w", domain.ErrServiceUnavailable)
	}

	target := net.JoinHostPort(
		cfg.GRPCClient.NotesService.Host,
		strconv.Itoa(cfg.GRPCClient.NotesService.Port),
	)

	var (
		dialCtx = ctx
		cancel  context.CancelFunc
	)
	if cfg.GRPCClient.NotesService.ConnectTimeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, cfg.GRPCClient.NotesService.ConnectTimeout)
		defer cancel()
	}

	conn, err := newClientConn(
		dialCtx,
		target,
		clientDialOptions(cfg)...,
	)
	if err != nil {
		return nil, fmt.Errorf("connect notes service: %w", err)
	}

	return new(NotesClient{notesClient: notesv1.NewNoteServiceClient(conn), conn: conn}), nil
}

func (c *NotesClient) CreateNote(
	ctx context.Context,
	accessToken, title, content string,
) (*domain.Note, error) {
	resp, err := c.notesClient.CreateNote(
		outgoingContextWithToken(ctx, accessToken),
		new(notesv1.CreateNoteRequest{Title: title, Content: content}),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrNoteNotFound)
	}

	return noteFromProto(resp.GetNote()), nil
}

func (c *NotesClient) GetNote(
	ctx context.Context,
	accessToken, noteID string,
) (*domain.Note, error) {
	resp, err := c.notesClient.GetNote(
		outgoingContextWithToken(ctx, accessToken),
		new(notesv1.GetNoteRequest{NoteId: noteID}),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrNoteNotFound)
	}

	return noteFromProto(resp.GetNote()), nil
}

func (c *NotesClient) ListNotes(
	ctx context.Context,
	accessToken string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	resp, err := c.notesClient.ListNotes(
		outgoingContextWithToken(ctx, accessToken),
		new(notesv1.ListNotesRequest{
			Limit:  clampInt32(limit),
			Offset: clampInt32(offset),
		}),
	)
	if err != nil {
		return nil, 0, statusToDomain(err, domain.ErrNoteNotFound)
	}

	notes := make([]*domain.Note, 0, len(resp.GetNotes()))
	for _, protoNote := range resp.GetNotes() {
		notes = append(notes, noteFromProto(protoNote))
	}

	return notes, int(resp.GetTotalCount()), nil
}

func (c *NotesClient) UpdateNote(
	ctx context.Context,
	accessToken, noteID string,
	title, content *string,
) (*domain.Note, error) {
	resp, err := c.notesClient.UpdateNote(
		outgoingContextWithToken(ctx, accessToken),
		new(notesv1.UpdateNoteRequest{NoteId: noteID, Title: title, Content: content}),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrNoteNotFound)
	}

	return noteFromProto(resp.GetNote()), nil
}

func (c *NotesClient) DeleteNote(ctx context.Context, accessToken, noteID string) error {
	_, err := c.notesClient.DeleteNote(
		outgoingContextWithToken(ctx, accessToken),
		new(notesv1.DeleteNoteRequest{NoteId: noteID}),
	)
	if err != nil {
		return statusToDomain(err, domain.ErrNoteNotFound)
	}

	return nil
}

func (c *NotesClient) Close() error {
	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	if err != nil {
		return fmt.Errorf("close notes grpc connection: %w", err)
	}

	return nil
}

func NewAuthClient(ctx context.Context, cfg *config.Config) (*AuthClient, error) {
	if cfg.GRPCClient == nil {
		return nil, fmt.Errorf("auth service: %w", domain.ErrServiceUnavailable)
	}

	target := net.JoinHostPort(
		cfg.GRPCClient.AuthService.Host,
		strconv.Itoa(cfg.GRPCClient.AuthService.Port),
	)

	var (
		dialCtx = ctx
		cancel  context.CancelFunc
	)
	if cfg.GRPCClient.AuthService.ConnectTimeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, cfg.GRPCClient.AuthService.ConnectTimeout)
		defer cancel()
	}

	conn, err := newClientConn(
		dialCtx,
		target,
		clientDialOptions(cfg)...,
	)
	if err != nil {
		return nil, fmt.Errorf("connect auth service: %w", err)
	}

	return new(
		AuthClient{
			authClient: authv1.NewAuthServiceClient(conn),
			userClient: authv1.NewUserServiceClient(conn),
			conn:       conn,
		},
	), nil
}

func (c *AuthClient) Register(
	ctx context.Context,
	email, username, password string,
) (*domain.TokenPair, error) {
	resp, err := c.authClient.Register(
		ctx,
		new(authv1.RegisterRequest{Email: email, Username: username, Password: password}),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrUserNotFound)
	}

	return tokenPairFromAuth(
		resp.GetUserId(),
		resp.GetUsername(),
		resp.GetAccessToken(),
		resp.GetRefreshToken(),
		resp.GetExpiresAt().AsTime(),
	), nil
}

func (c *AuthClient) Login(
	ctx context.Context,
	email, password string,
) (*domain.TokenPair, error) {
	resp, err := c.authClient.Login(ctx, new(authv1.LoginRequest{Email: email, Password: password}))
	if err != nil {
		return nil, statusToDomain(err, domain.ErrUserNotFound)
	}

	return tokenPairFromAuth(
		resp.GetUserId(),
		resp.GetUsername(),
		resp.GetAccessToken(),
		resp.GetRefreshToken(),
		resp.GetExpiresAt().AsTime(),
	), nil
}

func (c *AuthClient) RefreshTokens(
	ctx context.Context,
	refreshToken string,
) (*domain.TokenPair, error) {
	resp, err := c.authClient.RefreshTokens(
		ctx,
		new(authv1.RefreshTokensRequest{RefreshToken: refreshToken}),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrUserNotFound)
	}

	return tokenPairFromAuth(
		resp.GetUserId(),
		resp.GetUsername(),
		resp.GetAccessToken(),
		resp.GetRefreshToken(),
		resp.GetExpiresAt().AsTime(),
	), nil
}

func (c *AuthClient) Logout(ctx context.Context, refreshToken string) error {
	_, err := c.authClient.Logout(ctx, new(authv1.LogoutRequest{RefreshToken: refreshToken}))
	if err != nil {
		return statusToDomain(err, domain.ErrUserNotFound)
	}

	return nil
}

func (c *AuthClient) GetUserProfile(
	ctx context.Context,
	accessToken string,
) (*domain.User, error) {
	resp, err := c.userClient.GetUserProfile(
		outgoingContextWithToken(ctx, accessToken),
		new(emptypb.Empty),
	)
	if err != nil {
		return nil, statusToDomain(err, domain.ErrUserNotFound)
	}

	return new(domain.User{
		ID:           resp.GetUserId(),
		Email:        resp.GetEmail(),
		Username:     resp.GetUsername(),
		PasswordHash: "",
		CreatedAt:    resp.GetCreatedAt().AsTime(),
		UpdatedAt:    time.Time{},
	}), nil
}

func (c *AuthClient) Close() error {
	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	if err != nil {
		return fmt.Errorf("close auth grpc connection: %w", err)
	}

	return nil
}

func withClientTransport(cfg *config.Config) []grpc.DialOption {
	if cfg.GRPCClient == nil || cfg.GRPCClient.Insecure {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}

	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
	}
}

func clampInt32(value int) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}

	if value < 0 {
		return 0
	}

	return int32(value)
}
