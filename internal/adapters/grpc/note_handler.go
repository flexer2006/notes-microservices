package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

type NoteHandler struct {
	notesv1.UnimplementedNoteServiceServer

	noteUseCase ports.NoteUseCase
}

func NewNoteHandler(noteUseCase ports.NoteUseCase) *NoteHandler {
	return new(NoteHandler{
		UnimplementedNoteServiceServer: notesv1.UnimplementedNoteServiceServer{},
		noteUseCase:                    noteUseCase,
	})
}

func (h *NoteHandler) CreateNote(
	ctx context.Context,
	req *notesv1.CreateNoteRequest,
) (*notesv1.NoteResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	note, err := h.noteUseCase.CreateNote(ctx, userID, req.GetTitle(), req.GetContent())
	if err != nil {
		return nil, notesStatusFromError(err)
	}

	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) GetNote(
	ctx context.Context,
	req *notesv1.GetNoteRequest,
) (*notesv1.NoteResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	note, err := h.noteUseCase.GetNote(ctx, userID, req.GetNoteId())
	if err != nil {
		return nil, notesStatusFromError(err)
	}

	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) ListNotes(
	ctx context.Context,
	req *notesv1.ListNotesRequest,
) (*notesv1.ListNotesResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	notes, total, err := h.noteUseCase.ListNotes(
		ctx,
		userID,
		int(req.GetLimit()),
		int(req.GetOffset()),
	)
	if err != nil {
		return nil, notesStatusFromError(err)
	}

	responses := make([]*notesv1.Note, 0, len(notes))
	for _, n := range notes {
		responses = append(responses, noteToProto(n))
	}

	return new(notesv1.ListNotesResponse{
		Notes:      responses,
		TotalCount: clampNonNegative32(total),
		Offset:     req.GetOffset(),
		Limit:      req.GetLimit(),
	}), nil
}

func (h *NoteHandler) UpdateNote(
	ctx context.Context,
	req *notesv1.UpdateNoteRequest,
) (*notesv1.NoteResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	note, err := h.noteUseCase.UpdateNote(ctx, userID, req.GetNoteId(), req.Title, req.Content)
	if err != nil {
		return nil, notesStatusFromError(err)
	}

	return new(notesv1.NoteResponse{Note: noteToProto(note)}), nil
}

func (h *NoteHandler) DeleteNote(
	ctx context.Context,
	req *notesv1.DeleteNoteRequest,
) (*emptypb.Empty, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	deleteErr := h.noteUseCase.DeleteNote(ctx, userID, req.GetNoteId())
	if deleteErr != nil {
		return nil, notesStatusFromError(deleteErr)
	}

	return new(emptypb.Empty), nil
}

func (h *NoteHandler) RegisterService(server grpc.ServiceRegistrar) {
	notesv1.RegisterNoteServiceServer(server, h)
}

func clampNonNegative32(value int) int32 {
	const maxInt32 = int(^uint32(0) >> 1)
	if value > maxInt32 {
		return int32(maxInt32)
	}

	if value < 0 {
		return 0
	}

	return int32(value)
}
