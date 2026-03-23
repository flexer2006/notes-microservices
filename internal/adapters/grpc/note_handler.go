package grpc

import (
	"context"
	"math"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
	return new(NoteHandler{noteUseCase: noteUseCase})
}

func (h *NoteHandler) CreateNote(ctx context.Context, req *notesv1.CreateNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "NoteHandler.CreateNote")
	log.Debug(ctx, "create note request received")
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrAuthenticationRequired.Error())
	}
	noteID, err := h.noteUseCase.CreateNote(ctx, token, req.GetTitle(), req.GetContent())
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, grpcErrorFromDomain(err)
	}
	note, err := h.noteUseCase.GetNote(ctx, token, noteID)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, status.Error(codes.Internal, domain.ErrNoteCreatedButNotRetrieved.Error())
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) GetNote(ctx context.Context, req *notesv1.GetNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "NoteHandler.GetNote")
	log.Debug(ctx, "get note request received", zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrAuthenticationRequired.Error())
	}
	note, err := h.noteUseCase.GetNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, grpcErrorFromDomain(err)
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) ListNotes(ctx context.Context, req *notesv1.ListNotesRequest) (*notesv1.ListNotesResponse, error) {
	log := logger.Method(ctx, "NoteHandler.ListNotes")
	log.Debug(ctx, "list notes request received",
		zap.Int32("limit", req.GetLimit()),
		zap.Int32("offset", req.GetOffset()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrAuthenticationRequired.Error())
	}
	notes, total, err := h.noteUseCase.ListNotes(ctx, token, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, grpcErrorFromDomain(err)
	}
	responses := make([]*notesv1.Note, 0, len(notes))
	for _, n := range notes {
		responses = append(responses, noteToProto(n))
	}
	totalCount := int32(0)
	if total > 0 {
		if total > math.MaxInt32 {
			totalCount = int32(math.MaxInt32)
		} else {
			totalCount = int32(total)
		}
	}
	return new(notesv1.ListNotesResponse{
		Notes:      responses,
		TotalCount: totalCount,
		Offset:     req.GetOffset(),
		Limit:      req.GetLimit(),
	}), nil
}
func (h *NoteHandler) UpdateNote(ctx context.Context, req *notesv1.UpdateNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Method(ctx, "NoteHandler.UpdateNote")
	log.Debug(ctx, "update note request received", zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrAuthenticationRequired.Error())
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
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, grpcErrorFromDomain(err)
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) DeleteNote(ctx context.Context, req *notesv1.DeleteNoteRequest) (*emptypb.Empty, error) {
	log := logger.Method(ctx, "NoteHandler.DeleteNote")
	log.Debug(ctx, "delete note request received", zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, "failed to extract token", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrAuthenticationRequired.Error())
	}
	err = h.noteUseCase.DeleteNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequest.Error(), zap.Error(err))
		return nil, grpcErrorFromDomain(err)
	}
	return new(emptypb.Empty{}), nil
}

func (h *NoteHandler) RegisterService(server grpc.ServiceRegistrar) {
	notesv1.RegisterNoteServiceServer(server, h)
}
