package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	LogGetUserProfileRequest     = "processing user profile request"
	ErrUserNotFoundMsg           = "user not found"
	ErrInternalServiceMsg        = "internal service error"
	ErrMissingUserIDMsg          = "user ID missing in context"
	ErrGetUserProfileMsg         = "error getting user profile"
	ErrProfileRetrievalFailedMsg = "profile retrieval failed"
	ErrUnauthorizedAccessMsg     = "unauthorized access"
	LogMetadataNotFoundMsg       = "failed to get metadata from context"
	LogAuthHeaderMissingMsg      = "authorization header missing"
	LogInvalidTokenFormatMsg     = "invalid token format in authorization header"
	LogInvalidAccessTokenMsg     = "invalid access token"
)

var (
	ErrUserNotFound    = fmt.Errorf("%s", ErrUserNotFoundMsg)
	ErrInternalService = fmt.Errorf("%s", ErrInternalServiceMsg)
	ErrMissingUserID   = fmt.Errorf("%s", ErrMissingUserIDMsg)
)

type UserHandler struct {
	userUseCase ports.UserUseCase
	tokenSvc    ports.TokenService
	authv1.UnimplementedUserServiceServer
}

func NewUserHandler(userUseCase ports.UserUseCase, tokenSvc ports.TokenService) *UserHandler {
	return new(UserHandler{
		userUseCase: userUseCase,
		tokenSvc:    tokenSvc,
	})
}

func (h *UserHandler) GetUserProfile(ctx context.Context, _ *emptypb.Empty) (*authv1.UserProfileResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogGetUserProfileRequest)
	userID, ok := h.getUserIDFromContext(ctx)
	if !ok || userID == "" {
		log.Error(ctx, ErrMissingUserIDMsg)
		return nil, fmt.Errorf("%s: %w", ErrUnauthorizedAccessMsg, ErrMissingUserID)
	}
	user, err := h.userUseCase.GetUserProfile(ctx, userID)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", ErrGetUserProfileMsg, err))
		if errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("%s: %w", ErrProfileRetrievalFailedMsg, ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", ErrProfileRetrievalFailedMsg, ErrInternalService)
	}
	return new(authv1.UserProfileResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Username:  user.Username,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}), nil
}

type ServiceRegistrar interface {
	RegisterService(desc *grpc.ServiceDesc, impl any)
}

func (h *UserHandler) RegisterService(server ServiceRegistrar) {
	authv1.RegisterUserServiceServer(server, h)
}

func (h *UserHandler) getUserIDFromContext(ctx context.Context) (string, bool) {
	log := logger.Log(ctx)
	mda, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Debug(ctx, LogMetadataNotFoundMsg)
		return "", false
	}
	authHeader := mda.Get("authorization")
	if len(authHeader) == 0 {
		log.Debug(ctx, LogAuthHeaderMissingMsg)
		return "", false
	}
	tokenString := strings.TrimPrefix(authHeader[0], "Bearer ")
	if tokenString == authHeader[0] {
		log.Debug(ctx, LogInvalidTokenFormatMsg)
		return "", false
	}
	userID, err := h.tokenSvc.ValidateAccessToken(ctx, tokenString)
	if err != nil {
		log.Debug(ctx, LogInvalidAccessTokenMsg, zap.Error(err))
		return "", false
	}
	return userID, true
}

const (
	LogServerStarting = "Starting gRPC server"
	LogServerStarted  = "gRPC server started"
	LogServerStopping = "Stopping gRPC server"
	LogServerStopped  = "gRPC server stopped"
	ErrServerStart    = "failed to start gRPC server"
)

type Server struct {
	cfg    *config.GRPCConfig
	server *grpc.Server
}

func New(cfg *config.GRPCConfig) *Server {
	return new(Server{
		cfg:    cfg,
		server: grpc.NewServer(),
	})
}

func (s *Server) Start(ctx context.Context) error {
	log := logger.Log(ctx)
	address := s.cfg.GetAddress()
	log.Info(ctx, LogServerStarting, zap.String("address", address))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Error(ctx, ErrServerStart, zap.Error(err))
		return fmt.Errorf("%s: %w", ErrServerStart, err)
	}
	reflection.Register(s.server)
	go func() {
		if err := s.server.Serve(listener); err != nil {
			log.Error(ctx, ErrServerStart, zap.Error(err))
		}
	}()
	log.Info(ctx, LogServerStarted, zap.String("address", address))
	return nil
}

func (s *Server) Stop(ctx context.Context) {
	log := logger.Log(ctx)
	log.Info(ctx, "stopping gRPC server")
	s.server.GracefulStop()
}

func (s *Server) RegisterService(registerFn func(server *grpc.Server)) {
	registerFn(s.server)
}

func (s *Server) RegisterGRPCService(desc *grpc.ServiceDesc, impl any) {
	s.server.RegisterService(desc, impl)
}

//nolint:gosec
const (
	LogRegisterRequest           = "processing register request"
	LogLoginRequest              = "processing login request"
	LogRefreshTokenRequest       = "processing refresh token request"
	LogLogoutRequest             = "processing logout request"
	ErrInvalidCredentialsMsg     = "invalid credentials" // #nosec G101
	ErrInvalidTokenMsg           = "invalid refresh token"
	ErrAuthServiceInternalMsg    = "internal authentication service error"
	ErrInvalidRequestMsg         = "invalid request data"
	ErrUserAlreadyExistsMsg      = "user already exists"
	ErrLogoutFailedTokenMsg      = "logout failed with invalid token"
	ErrLogoutOperationFailedMsg  = "logout operation failed"
	ErrRefreshTokensMsg          = "refreshTokens error"
	ErrMissingRefreshTokenMsg    = "missing refresh token"
	ErrMissingRefreshLogoutMsg   = "missing refresh token for logout"
	ErrRegisterMsg               = "register error"
	ErrUserRegistrationFailedMsg = "user registration failed"
	ErrAuthServiceErrorMsg       = "auth service error"
	ErrLoginMsg                  = "login error"
	ErrAuthenticationFailedMsg   = "authentication failed"
	ErrTokenRefreshFailedMsg     = "token refresh failed"
	ErrLogoutMsg                 = "logout error"
	ErrInvalidLoginParamsMsg     = "invalid login parameters"
	ErrInvalidRequestParamsMsg   = "invalid request parameters"
)

var (
	ErrInvalidRequest      = fmt.Errorf("%s", ErrInvalidRequestMsg)
	ErrInvalidCredentials  = fmt.Errorf("%s", ErrInvalidCredentialsMsg)
	ErrInvalidToken        = fmt.Errorf("%s", ErrInvalidTokenMsg)
	ErrAuthServiceInternal = fmt.Errorf("%s", ErrAuthServiceInternalMsg)
	ErrUserAlreadyExists   = fmt.Errorf("%s", ErrUserAlreadyExistsMsg)
)

type AuthHandler struct {
	authUseCase ports.AuthUseCase
	authv1.UnimplementedAuthServiceServer
}

func NewAuthHandler(authUseCase ports.AuthUseCase) *AuthHandler {
	return new(AuthHandler{authUseCase: authUseCase})
}

func (h *AuthHandler) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogRegisterRequest,
		zap.String("email", req.Email),
		zap.String("username", req.Username))
	if req.Email == "" || req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", ErrInvalidRequestParamsMsg, ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Register(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", ErrRegisterMsg, err))
		switch err.Error() {
		case ErrUserAlreadyExistsMsg:
			return nil, fmt.Errorf("%s: %w", ErrUserRegistrationFailedMsg, ErrUserAlreadyExists)
		default:
			return nil, fmt.Errorf("%s: %w", ErrAuthServiceErrorMsg, ErrAuthServiceInternal)
		}
	}
	return new(authv1.RegisterResponse{
		UserId:       tokenPair.UserID,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogLoginRequest, zap.String("email", req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("%s: %w", ErrInvalidLoginParamsMsg, ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.Login(ctx, req.Email, req.Password)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", ErrLoginMsg, err))
		return nil, fmt.Errorf("%s: %w", ErrAuthenticationFailedMsg, ErrInvalidCredentials)
	}
	return new(authv1.LoginResponse{
		UserId:       tokenPair.UserID,
		Username:     tokenPair.Username,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) RefreshTokens(ctx context.Context, req *authv1.RefreshTokensRequest) (*authv1.RefreshTokensResponse, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogRefreshTokenRequest)
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", ErrMissingRefreshTokenMsg, ErrInvalidRequest)
	}
	tokenPair, err := h.authUseCase.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", ErrRefreshTokensMsg, err))
		return nil, fmt.Errorf("%s: %w", ErrTokenRefreshFailedMsg, ErrInvalidToken)
	}
	return new(authv1.RefreshTokensResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    timestamppb.New(tokenPair.ExpiresAt),
	}), nil
}

func (h *AuthHandler) Logout(ctx context.Context, req *authv1.LogoutRequest) (*emptypb.Empty, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogLogoutRequest)
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("%s: %w", ErrMissingRefreshLogoutMsg, ErrInvalidRequest)
	}
	err := h.authUseCase.Logout(ctx, req.RefreshToken)
	if err != nil {
		log.Error(ctx, fmt.Sprintf("%s: %v", ErrLogoutMsg, err))
		if err.Error() == ErrInvalidTokenMsg {
			return nil, fmt.Errorf("%s: %w", ErrLogoutFailedTokenMsg, ErrInvalidToken)
		}
		return nil, fmt.Errorf("%s: %w", ErrLogoutOperationFailedMsg, ErrAuthServiceInternal)
	}
	return new(emptypb.Empty{}), nil
}

func (h *AuthHandler) RegisterService(server ServiceRegistrar) {
	authv1.RegisterAuthServiceServer(server, h)
}

func wrapGrpcError(code codes.Code, message string) error {
	return fmt.Errorf("gRPC error: %w", status.Error(code, message))
}

type NoteUseCase interface {
	CreateNote(ctx context.Context, token, title, content string) (string, error)
	GetNote(ctx context.Context, token, noteID string) (*domain.Note, error)
	ListNotes(ctx context.Context, token string, limit, offset int) ([]*domain.Note, int, error)
	UpdateNote(ctx context.Context, token, noteID, title, content string) (*domain.Note, error)
	DeleteNote(ctx context.Context, token, noteID string) error
}

type NoteHandler struct {
	noteUseCase NoteUseCase
	notesv1.UnimplementedNoteServiceServer
}

func NewNoteHandler(noteUseCase NoteUseCase) *NoteHandler {
	return new(NoteHandler{
		noteUseCase: noteUseCase,
	})
}

func ExtractToken(ctx context.Context) (string, error) {
	mtd, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", domain.ErrMetadataNotFound
	}
	logger.Log(ctx).Debug(ctx, "Received metadata", zap.Any("metadata", mtd))
	values := mtd.Get("authorization")
	if len(values) == 0 {
		return "", domain.ErrAuthHeaderNotFound
	}
	authHeader := values[0]
	logger.Log(ctx).Debug(ctx, "Received authorization header", zap.String("auth_header", authHeader))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:], nil
	}
	return authHeader, nil
}

func (h *NoteHandler) CreateNote(ctx context.Context, req *notesv1.CreateNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.CreateNote"))
	log.Debug(ctx, "create note request received")
	token, err := ExtractToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, wrapGrpcError(codes.Unauthenticated, "authentication required")
	}
	noteID, err := h.noteUseCase.CreateNote(ctx, token, req.GetTitle(), req.GetContent())
	if err != nil {
		log.Error(ctx, "failed to create note", zap.Error(err))
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, wrapGrpcError(codes.Unauthenticated, "invalid or expired token")
		}
		return nil, wrapGrpcError(codes.Internal, "failed to create note")
	}
	note, err := h.noteUseCase.GetNote(ctx, token, noteID)
	if err != nil {
		log.Error(ctx, "failed to get created note", zap.Error(err))
		return nil, wrapGrpcError(codes.Internal, "note was created but could not be retrieved")
	}
	return new(notesv1.NoteResponse{
		Note: new(notesv1.Note{
			NoteId:    note.ID,
			UserId:    note.UserID,
			Title:     note.Title,
			Content:   note.Content,
			CreatedAt: timestamppb.New(note.CreatedAt),
			UpdatedAt: timestamppb.New(note.UpdatedAt),
		}),
	}), nil
}

func (h *NoteHandler) GetNote(ctx context.Context, req *notesv1.GetNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.GetNote"))
	log.Debug(ctx, "get note request received", zap.String("noteID", req.GetNoteId()))
	token, err := ExtractToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, wrapGrpcError(codes.Unauthenticated, "authentication required")
	}
	note, err := h.noteUseCase.GetNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, "failed to get note", zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, wrapGrpcError(codes.Unauthenticated, "invalid or expired token")
		case errors.Is(err, domain.ErrNotFound):
			return nil, wrapGrpcError(codes.NotFound, "note not found")
		default:
			return nil, wrapGrpcError(codes.Internal, "failed to get note")
		}
	}
	return new(notesv1.NoteResponse{
		Note: new(notesv1.Note{
			NoteId:    note.ID,
			UserId:    note.UserID,
			Title:     note.Title,
			Content:   note.Content,
			CreatedAt: timestamppb.New(note.CreatedAt),
			UpdatedAt: timestamppb.New(note.UpdatedAt),
		}),
	}), nil
}

func (h *NoteHandler) ListNotes(ctx context.Context, req *notesv1.ListNotesRequest) (*notesv1.ListNotesResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.ListNotes"))
	log.Debug(ctx, "list notes request received",
		zap.Int32("limit", req.GetLimit()),
		zap.Int32("offset", req.GetOffset()))
	token, err := ExtractToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, wrapGrpcError(codes.Unauthenticated, "authentication required")
	}
	notes, total, err := h.noteUseCase.ListNotes(ctx, token, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		log.Error(ctx, "failed to list notes", zap.Error(err))
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, wrapGrpcError(codes.Unauthenticated, "invalid or expired token")
		}
		return nil, wrapGrpcError(codes.Internal, "failed to list notes")
	}
	noteResponses := make([]*notesv1.Note, 0, len(notes))
	for _, note := range notes {
		noteResponses = append(noteResponses, new(notesv1.Note{
			NoteId:    note.ID,
			UserId:    note.UserID,
			Title:     note.Title,
			Content:   note.Content,
			CreatedAt: timestamppb.New(note.CreatedAt),
			UpdatedAt: timestamppb.New(note.UpdatedAt),
		}))
	}
	var totalCount int32
	switch {
	case total <= 0:
		totalCount = 0
	case total > 2147483647:
		totalCount = 2147483647
	default:
		totalCount = int32(total)
	}
	return new(notesv1.ListNotesResponse{
		Notes:      noteResponses,
		TotalCount: totalCount,
		Offset:     req.GetOffset(),
		Limit:      req.GetLimit(),
	}), nil
}

func (h *NoteHandler) UpdateNote(ctx context.Context, req *notesv1.UpdateNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.UpdateNote"))
	log.Debug(ctx, "update note request received", zap.String("noteID", req.GetNoteId()))
	token, err := ExtractToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, wrapGrpcError(codes.Unauthenticated, "authentication required")
	}
	var title, content string
	if req.Title != nil {
		title = *req.Title
	}
	if req.Content != nil {
		content = *req.Content
	}
	note, err := h.noteUseCase.UpdateNote(ctx, token, req.GetNoteId(), title, content)
	if err != nil {
		log.Error(ctx, "failed to update note", zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, wrapGrpcError(codes.Unauthenticated, "invalid or expired token")
		case errors.Is(err, domain.ErrNotFound):
			return nil, wrapGrpcError(codes.NotFound, "note not found")
		default:
			return nil, wrapGrpcError(codes.Internal, "failed to update note")
		}
	}
	return new(notesv1.NoteResponse{
		Note: &notesv1.Note{
			NoteId:    note.ID,
			UserId:    note.UserID,
			Title:     note.Title,
			Content:   note.Content,
			CreatedAt: timestamppb.New(note.CreatedAt),
			UpdatedAt: timestamppb.New(note.UpdatedAt),
		},
	}), nil
}

func (h *NoteHandler) DeleteNote(ctx context.Context, req *notesv1.DeleteNoteRequest) (*emptypb.Empty, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.DeleteNote"))
	log.Debug(ctx, "delete note request received", zap.String("noteID", req.GetNoteId()))
	token, err := ExtractToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, wrapGrpcError(codes.Unauthenticated, "authentication required")
	}
	err = h.noteUseCase.DeleteNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, "failed to delete note", zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, wrapGrpcError(codes.Unauthenticated, "invalid or expired token")
		case errors.Is(err, domain.ErrNotFound):
			return nil, wrapGrpcError(codes.NotFound, "note not found")
		default:
			return nil, wrapGrpcError(codes.Internal, "failed to delete note")
		}
	}
	return new(emptypb.Empty{}), nil
}

const (
	LogMethodCreateNote     = "CreateNote"
	LogMethodGetNote        = "GetNote"
	LogMethodListNotes      = "ListNotes"
	LogMethodUpdateNote     = "UpdateNote"
	LogMethodDeleteNote     = "DeleteNote"
	ErrorFailedToCreateNote = "failed to create note"
	ErrorFailedToGetNote    = "failed to get note"
	ErrorFailedToListNotes  = "failed to list notes"
	ErrorFailedToUpdateNote = "failed to update note"
	ErrorFailedToDeleteNote = "failed to delete note"
)

var ErrNotesServiceConnectionTimeout = errors.New("connection timeout: failed to connect to notes service")

type NotesClient struct {
	notesClient notesv1.NoteServiceClient
	conn        *grpc.ClientConn
}

func NewNotesClient(ctx context.Context, cfg *config.GRPCClientConfig) (ports.NotesServiceClient, error) {
	conn, err := grpc.DialContext(ctx,
		cfg.NotesService.GetAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to notes service: %w", err)
	}
	return new(NotesClient{
		notesClient: notesv1.NewNoteServiceClient(conn),
		conn:        conn,
	}), nil
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

func (c *NotesClient) CreateNote(ctx context.Context, title, content string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodCreateNote))
	req := new(notesv1.CreateNoteRequest{
		Title:   title,
		Content: content,
	})
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	log.Debug(ctx, "Sending token to notes service",
		zap.String("raw_token", token))
	formattedToken := formatAuthorizationToken(token)
	log.Debug(ctx, "Token after formatting",
		zap.String("formatted_token", formattedToken))
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formattedToken))
	resp, err := c.notesClient.CreateNote(outCtx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToCreateNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToCreateNote, err)
	}
	return resp, nil
}

func (c *NotesClient) GetNote(ctx context.Context, noteID string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodGetNote))

	req := &notesv1.GetNoteRequest{
		NoteId: noteID,
	}
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	log.Debug(ctx, "Sending token to notes service",
		zap.String("raw_token", token))
	formattedToken := formatAuthorizationToken(token)
	log.Debug(ctx, "Token after formatting",
		zap.String("formatted_token", formattedToken))
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formattedToken))
	resp, err := c.notesClient.GetNote(outCtx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToGetNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToGetNote, err)
	}
	return resp, nil
}

func (c *NotesClient) ListNotes(ctx context.Context, limit, offset int32) (*notesv1.ListNotesResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodListNotes))
	req := &notesv1.ListNotesRequest{
		Limit:  limit,
		Offset: offset,
	}
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	log.Debug(ctx, "Sending token to notes service",
		zap.String("raw_token", token))
	formattedToken := formatAuthorizationToken(token)
	log.Debug(ctx, "Token after formatting",
		zap.String("formatted_token", formattedToken))
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formattedToken))
	resp, err := c.notesClient.ListNotes(outCtx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToListNotes, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToListNotes, err)
	}
	return resp, nil
}

func (c *NotesClient) UpdateNote(ctx context.Context, noteID string, title, content *string) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodUpdateNote))
	req := new(notesv1.UpdateNoteRequest{
		NoteId:  noteID,
		Title:   title,
		Content: content,
	})
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	log.Debug(ctx, "Sending token to notes service",
		zap.String("raw_token", token))
	formattedToken := formatAuthorizationToken(token)
	log.Debug(ctx, "Token after formatting",
		zap.String("formatted_token", formattedToken))
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formattedToken))
	resp, err := c.notesClient.UpdateNote(outCtx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToUpdateNote, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToUpdateNote, err)
	}
	return resp, nil
}

func (c *NotesClient) DeleteNote(ctx context.Context, noteID string) error {
	log := logger.Log(ctx).With(zap.String("method", LogMethodDeleteNote))
	req := &notesv1.DeleteNoteRequest{
		NoteId: noteID,
	}
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	log.Debug(ctx, "Sending token to notes service",
		zap.String("raw_token", token))
	formattedToken := formatAuthorizationToken(token)
	log.Debug(ctx, "Token after formatting",
		zap.String("formatted_token", formattedToken))
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", formattedToken))
	_, err := c.notesClient.DeleteNote(outCtx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToDeleteNote, zap.Error(err))
		return fmt.Errorf("%s: %w", ErrorFailedToDeleteNote, err)
	}
	return nil
}

func (c *NotesClient) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return fmt.Errorf("failed to close grpc connection: %w", err)
		}
	}
	return nil
}

const (
	LogMethodRegister          = "Register"
	LogMethodLogin             = "Login"
	LogMethodRefreshTokens     = "RefreshTokens"
	LogMethodLogout            = "Logout"
	LogMethodGetUserProfile    = "GetUserProfile"
	ErrorFailedToRegister      = "failed to register user"
	ErrorFailedToLogin         = "failed to login"
	ErrorFailedToRefreshTokens = "failed to update tokens"
	ErrorFailedToLogout        = "failed to logout"
	ErrorFailedToGetProfile    = "failed to get user profile"
)

var ErrAuthServiceConnectionTimeout = errors.New("connection timeout: failed to connect to auth service")

type AuthClient struct {
	authClient authv1.AuthServiceClient
	userClient authv1.UserServiceClient
	conn       *grpc.ClientConn
}

func NewAuthClient(ctx context.Context, cfg *config.GRPCClientConfig) (ports.AuthServiceClient, error) {
	conn, err := grpc.DialContext(ctx,
		cfg.AuthService.GetAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to auth service: %w", err)
	}
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(ctx, state) {
			closeErr := conn.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("failed to close connection: %w", closeErr)
			}
			return nil, ErrAuthServiceConnectionTimeout
		}
	}
	return new(AuthClient{
		authClient: authv1.NewAuthServiceClient(conn),
		userClient: authv1.NewUserServiceClient(conn),
		conn:       conn,
	}), nil
}

func (c *AuthClient) Register(ctx context.Context, email, username, password string) (*authv1.RegisterResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodRegister))
	req := new(authv1.RegisterRequest{
		Email:    email,
		Username: username,
		Password: password,
	})
	resp, err := c.authClient.Register(ctx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToRegister, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToRegister, err)
	}
	return resp, nil
}

func (c *AuthClient) Login(ctx context.Context, email, password string) (*authv1.LoginResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodLogin))
	req := &authv1.LoginRequest{
		Email:    email,
		Password: password,
	}
	resp, err := c.authClient.Login(ctx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToLogin, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToLogin, err)
	}
	return resp, nil
}

func (c *AuthClient) RefreshTokens(ctx context.Context, refreshToken string) (*authv1.RefreshTokensResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodRefreshTokens))
	req := &authv1.RefreshTokensRequest{
		RefreshToken: refreshToken,
	}
	resp, err := c.authClient.RefreshTokens(ctx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToRefreshTokens, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToRefreshTokens, err)
	}
	return resp, nil
}

func (c *AuthClient) Logout(ctx context.Context, refreshToken string) error {
	log := logger.Log(ctx).With(zap.String("method", LogMethodLogout))
	req := &authv1.LogoutRequest{
		RefreshToken: refreshToken,
	}
	_, err := c.authClient.Logout(ctx, req)
	if err != nil {
		log.Error(ctx, ErrorFailedToLogout, zap.Error(err))
		return fmt.Errorf("%s: %w", ErrorFailedToLogout, err)
	}
	return nil
}

func (c *AuthClient) GetUserProfile(ctx context.Context) (*authv1.UserProfileResponse, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodGetUserProfile))
	md, ok := metadata.FromIncomingContext(ctx)
	token := ""
	if ok && len(md["authorization"]) > 0 {
		token = md["authorization"][0]
	}
	outCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", token))
	resp, err := c.userClient.GetUserProfile(outCtx, &emptypb.Empty{})
	if err != nil {
		log.Error(ctx, ErrorFailedToGetProfile, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrorFailedToGetProfile, err)
	}
	return resp, nil
}
