package grpc

import (
	"context"
	"fmt"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"

	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

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

type NotesClient struct {
	notesClient notesv1.NoteServiceClient
	conn        *grpc.ClientConn
}

func NewNotesClient(ctx context.Context, cfg *config.Config) (*NotesClient, error) {
	if cfg.GRPCClient == nil {
		return nil, fmt.Errorf("%s: grpc client config is missing", domain.ErrorFailedToConnectNotesSvc)
	}
	target := fmt.Sprintf("%s:%d", cfg.GRPCClient.NotesService.Host, cfg.GRPCClient.NotesService.Port)
	conn, err := newClientConn(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToConnectNotesSvc, err)
	}
	return new(NotesClient{notesClient: notesv1.NewNoteServiceClient(conn), conn: conn}), nil
}

func (c *NotesClient) CreateNote(ctx context.Context, title, content string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodCreateNote))
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.CreateNote(outCtx, &notesv1.CreateNoteRequest{Title: title, Content: content})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToCreateNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToCreateNote, err)
	}
	return resp, nil
}

func (c *NotesClient) GetNote(ctx context.Context, noteID string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodGetNote))
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.GetNote(outCtx, &notesv1.GetNoteRequest{NoteId: noteID})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToGetNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToGetNote, err)
	}
	return resp, nil
}

func (c *NotesClient) ListNotes(ctx context.Context, limit, offset int32) (*notesv1.ListNotesResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodListNotes))
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.ListNotes(outCtx, &notesv1.ListNotesRequest{Limit: limit, Offset: offset})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToListNotes, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToListNotes, err)
	}
	return resp, nil
}

func (c *NotesClient) UpdateNote(ctx context.Context, noteID string, title, content *string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodUpdateNote))
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.UpdateNote(outCtx, &notesv1.UpdateNoteRequest{NoteId: noteID, Title: title, Content: content})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToUpdateNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToUpdateNote, err)
	}
	return resp, nil
}

func (c *NotesClient) DeleteNote(ctx context.Context, noteID string) error {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodDeleteNote))
	outCtx := outgoingContextWithAuth(ctx)
	_, err := c.notesClient.DeleteNote(outCtx, &notesv1.DeleteNoteRequest{NoteId: noteID})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToDeleteNote, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrorFailedToDeleteNote, err)
	}
	return nil
}

func (c *NotesClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("%s: %w", domain.ErrorFailedToCloseGrpcConn, err)
	}
	return nil
}

type AuthClient struct {
	authClient authv1.AuthServiceClient
	userClient authv1.UserServiceClient
	conn       *grpc.ClientConn
}

func NewAuthClient(ctx context.Context, cfg *config.Config) (*AuthClient, error) {
	if cfg.GRPCClient == nil {
		return nil, fmt.Errorf("%s: grpc client config is missing", domain.ErrorFailedToConnectAuthSvc)
	}
	target := fmt.Sprintf("%s:%d", cfg.GRPCClient.AuthService.Host, cfg.GRPCClient.AuthService.Port)
	conn, err := newClientConn(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToConnectAuthSvc, err)
	}
	return &AuthClient{authClient: authv1.NewAuthServiceClient(conn), userClient: authv1.NewUserServiceClient(conn), conn: conn}, nil
}

func (c *AuthClient) Register(ctx context.Context, email, username, password string) (*authv1.RegisterResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRegister))
	resp, err := c.authClient.Register(ctx, &authv1.RegisterRequest{Email: email, Username: username, Password: password})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToRegister, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToRegister, err)
	}
	return resp, nil
}

func (c *AuthClient) Login(ctx context.Context, email, password string) (*authv1.LoginResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogin))
	resp, err := c.authClient.Login(ctx, &authv1.LoginRequest{Email: email, Password: password})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToLogin, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToLogin, err)
	}
	return resp, nil
}

func (c *AuthClient) RefreshTokens(ctx context.Context, refreshToken string) (*authv1.RefreshTokensResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodRefreshTokens))
	resp, err := c.authClient.RefreshTokens(ctx, &authv1.RefreshTokensRequest{RefreshToken: refreshToken})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToRefreshTokens, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToRefreshTokens, err)
	}
	return resp, nil
}

func (c *AuthClient) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodLogout))
	_, err := c.authClient.Logout(ctx, &authv1.LogoutRequest{RefreshToken: refreshToken})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToLogout, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrorFailedToLogout, err)
	}
	return nil
}

func (c *AuthClient) GetUserProfile(ctx context.Context) (*authv1.UserProfileResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodGetUserProfile))
	resp, err := c.userClient.GetUserProfile(outgoingContextWithAuth(ctx), &emptypb.Empty{})
	if err != nil {
		log.Error(ctx, domain.ErrorFailedToGetProfile, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrorFailedToGetProfile, err)
	}
	return resp, nil
}

func (c *AuthClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("%s: %w", domain.ErrorFailedToCloseGrpcConn, err)
	}
	return nil
}
