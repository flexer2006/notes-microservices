package grpc

import (
	"context"
	"fmt"
	"net"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"

	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
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
		return nil, fmt.Errorf("%s: grpc client config is missing", domain.ErrFailedToConnectNotesSvc.Error())
	}
	target := net.JoinHostPort(cfg.GRPCClient.NotesService.Host, fmt.Sprint(cfg.GRPCClient.NotesService.Port))
	conn, err := newClientConn(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToConnectNotesSvc.Error(), err)
	}
	return new(NotesClient{notesClient: notesv1.NewNoteServiceClient(conn), conn: conn}), nil
}

func (c *NotesClient) CreateNote(ctx context.Context, title, content string) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "CreateNote")
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.CreateNote(outCtx, new(notesv1.CreateNoteRequest{Title: title, Content: content}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToCreateNote.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToCreateNote.Error(), err)
	}
	return resp, nil
}

func (c *NotesClient) GetNote(ctx context.Context, noteID string) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "GetNote")
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.GetNote(outCtx, new(notesv1.GetNoteRequest{NoteId: noteID}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToGetNote.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToGetNote.Error(), err)
	}
	return resp, nil
}

func (c *NotesClient) ListNotes(ctx context.Context, limit, offset int32) (*notesv1.ListNotesResponse, error) {
	log := logger.Method(ctx, "ListNotes")
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.ListNotes(outCtx, new(notesv1.ListNotesRequest{Limit: limit, Offset: offset}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToListNotes.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToListNotes.Error(), err)
	}
	return resp, nil
}

func (c *NotesClient) UpdateNote(ctx context.Context, noteID string, title, content *string) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "UpdateNote")
	outCtx := outgoingContextWithAuth(ctx)
	resp, err := c.notesClient.UpdateNote(outCtx, new(notesv1.UpdateNoteRequest{NoteId: noteID, Title: title, Content: content}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToUpdateNote.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToUpdateNote.Error(), err)
	}
	return resp, nil
}

func (c *NotesClient) DeleteNote(ctx context.Context, noteID string) error {
	log := logger.Method(ctx, "DeleteNote")
	outCtx := outgoingContextWithAuth(ctx)
	_, err := c.notesClient.DeleteNote(outCtx, new(notesv1.DeleteNoteRequest{NoteId: noteID}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToDeleteNote.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrFailedToDeleteNote.Error(), err)
	}
	return nil
}

func (c *NotesClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("%s: %w", domain.ErrFailedToCloseGrpcConn.Error(), err)
	}
	return nil
}

func NewAuthClient(ctx context.Context, cfg *config.Config) (*AuthClient, error) {
	if cfg.GRPCClient == nil {
		return nil, fmt.Errorf("%s: grpc client config is missing", domain.ErrFailedToConnectAuthSvc.Error())
	}
	target := fmt.Sprintf("%s:%d", cfg.GRPCClient.AuthService.Host, cfg.GRPCClient.AuthService.Port)
	conn, err := newClientConn(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToConnectAuthSvc.Error(), err)
	}
	return new(AuthClient{authClient: authv1.NewAuthServiceClient(conn), userClient: authv1.NewUserServiceClient(conn), conn: conn}), nil
}

func (c *AuthClient) Register(ctx context.Context, email, username, password string) (*authv1.RegisterResponse, error) {
	log := logger.Method(ctx, "Register")
	resp, err := c.authClient.Register(ctx, new(authv1.RegisterRequest{Email: email, Username: username, Password: password}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToRegister.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToRegister.Error(), err)
	}
	return resp, nil
}

func (c *AuthClient) Login(ctx context.Context, email, password string) (*authv1.LoginResponse, error) {
	log := logger.Method(ctx, "Login")
	resp, err := c.authClient.Login(ctx, new(authv1.LoginRequest{Email: email, Password: password}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToLogin.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToLogin.Error(), err)
	}
	return resp, nil
}

func (c *AuthClient) RefreshTokens(ctx context.Context, refreshToken string) (*authv1.RefreshTokensResponse, error) {
	log := logger.Method(ctx, "RefreshTokens")
	resp, err := c.authClient.RefreshTokens(ctx, new(authv1.RefreshTokensRequest{RefreshToken: refreshToken}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToRefreshTokens.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToRefreshTokens.Error(), err)
	}
	return resp, nil
}

func (c *AuthClient) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Method(ctx, "Logout")
	_, err := c.authClient.Logout(ctx, new(authv1.LogoutRequest{RefreshToken: refreshToken}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToLogout.Error(), zap.Error(err))
		return fmt.Errorf("%s: %w", domain.ErrFailedToLogout.Error(), err)
	}
	return nil
}

func (c *AuthClient) GetUserProfile(ctx context.Context) (*authv1.UserProfileResponse, error) {
	log := logger.Method(ctx, "GetUserProfile")
	resp, err := c.userClient.GetUserProfile(outgoingContextWithAuth(ctx), new(emptypb.Empty{}))
	if err != nil {
		log.Error(ctx, domain.ErrFailedToGetProfile.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedToGetProfile.Error(), err)
	}
	return resp, nil
}

func (c *AuthClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("%s: %w", domain.ErrFailedToCloseGrpcConn.Error(), err)
	}
	return nil
}
