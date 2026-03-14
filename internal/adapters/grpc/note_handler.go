package grpc

import (
	"context"
	"errors"
	"math"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.CreateNote"))
	log.Debug(ctx, domain.LogCreateNoteRequestReceived)
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.LogFailedToExtractToken, zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrorAuthenticationRequired)
	}
	noteID, err := h.noteUseCase.CreateNote(ctx, token, req.GetTitle(), req.GetContent())
	if err != nil {
		log.Error(ctx, domain.LogErrorCreateNoteFailed, zap.Error(err))
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.Unauthenticated, domain.ErrorInvalidOrExpiredToken)
		}
		return nil, status.Error(codes.Internal, domain.ErrorFailedToCreateNote)
	}
	note, err := h.noteUseCase.GetNote(ctx, token, noteID)
	if err != nil {
		log.Error(ctx, domain.LogErrorGetCreatedNote, zap.Error(err))
		return nil, status.Error(codes.Internal, domain.ErrorNoteCreatedButNotRetrieved)
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) GetNote(ctx context.Context, req *notesv1.GetNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.GetNote"))
	log.Debug(ctx, domain.LogGetNoteRequestReceived, zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.LogFailedToExtractToken, zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrorAuthenticationRequired)
	}
	note, err := h.noteUseCase.GetNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, domain.LogErrorGetNoteFailed, zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, status.Error(codes.Unauthenticated, domain.ErrorInvalidOrExpiredToken)
		case errors.Is(err, domain.ErrNotFound):
			return nil, status.Error(codes.NotFound, domain.ErrorNoteNotFound)
		default:
			return nil, status.Error(codes.Internal, domain.ErrorFailedToGetNote)
		}
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) ListNotes(ctx context.Context, req *notesv1.ListNotesRequest) (*notesv1.ListNotesResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.ListNotes"))
	log.Debug(ctx, domain.LogListNotesRequestReceived,
		zap.Int32("limit", req.GetLimit()),
		zap.Int32("offset", req.GetOffset()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.LogFailedToExtractToken, zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrorAuthenticationRequired)
	}
	notes, total, err := h.noteUseCase.ListNotes(ctx, token, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		log.Error(ctx, domain.LogErrorListNotesFailed, zap.Error(err))
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.Unauthenticated, domain.ErrorInvalidOrExpiredToken)
		}
		return nil, status.Error(codes.Internal, domain.ErrorFailedToListNotes)
	}
	responses := make([]*notesv1.Note, 0, len(notes))
	for _, n := range notes {
		responses = append(responses, noteToProto(n))
	}
	totalCount := int32(total)
	if total < 0 {
		totalCount = 0
	} else if total > math.MaxInt32 {
		totalCount = int32(math.MaxInt32)
	}
	return new(notesv1.ListNotesResponse{
		Notes:      responses,
		TotalCount: totalCount,
		Offset:     req.GetOffset(),
		Limit:      req.GetLimit(),
	}), nil
}
func (h *NoteHandler) UpdateNote(ctx context.Context, req *notesv1.UpdateNoteRequest) (*notesv1.NoteResponse, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.UpdateNote"))
	log.Debug(ctx, domain.LogUpdateNoteRequestReceived, zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.LogFailedToExtractToken, zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrorAuthenticationRequired)
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
		log.Error(ctx, domain.LogErrorUpdateNoteFailed, zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, status.Error(codes.Unauthenticated, domain.ErrorInvalidOrExpiredToken)
		case errors.Is(err, domain.ErrNotFound):
			return nil, status.Error(codes.NotFound, domain.ErrorNoteNotFound)
		default:
			return nil, status.Error(codes.Internal, domain.ErrorFailedToUpdateNote)
		}
	}
	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) DeleteNote(ctx context.Context, req *notesv1.DeleteNoteRequest) (*emptypb.Empty, error) {
	log := logger.Log(ctx).With(zap.String("handler", "NoteHandler.DeleteNote"))
	log.Debug(ctx, domain.LogDeleteNoteRequestReceived, zap.String("noteID", req.GetNoteId()))
	token, err := extractBearerToken(ctx)
	if err != nil {
		log.Error(ctx, domain.LogFailedToExtractToken, zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, domain.ErrorAuthenticationRequired)
	}
	err = h.noteUseCase.DeleteNote(ctx, token, req.GetNoteId())
	if err != nil {
		log.Error(ctx, domain.LogErrorDeleteNoteFailed, zap.Error(err))
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			return nil, status.Error(codes.Unauthenticated, domain.ErrorInvalidOrExpiredToken)
		case errors.Is(err, domain.ErrNotFound):
			return nil, status.Error(codes.NotFound, domain.ErrorNoteNotFound)
		default:
			return nil, status.Error(codes.Internal, domain.ErrorFailedToDeleteNote)
		}
	}
	return new(emptypb.Empty{}), nil
}

func (h *NoteHandler) RegisterService(server grpc.ServiceRegistrar) {
	notesv1.RegisterNoteServiceServer(server, h)
}

func noteToProto(note *domain.Note) *notesv1.Note {
	if note == nil {
		return nil
	}
	return &notesv1.Note{
		NoteId:    note.ID,
		UserId:    note.UserID,
		Title:     note.Title,
		Content:   note.Content,
		CreatedAt: timestamppb.New(note.CreatedAt),
		UpdatedAt: timestamppb.New(note.UpdatedAt),
	}
}
