package http

import (
	"math"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type NotesHandler struct {
	notesService ports.NotesService
}
type CreateNoteRequest struct {
	Title   string `json:"title" validate:"required"`
	Content string `json:"content" validate:"required"`
}

type UpdateNoteRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

type Note struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NoteResponse struct {
	Note *Note `json:"note"`
}

type ListNotesResponse struct {
	Notes      []*Note `json:"notes"`
	TotalCount int32   `json:"total_count"`
	Offset     int32   `json:"offset"`
	Limit      int32   `json:"limit"`
}

func NewNotesHandler(notesService ports.NotesService) *NotesHandler {
	return new(NotesHandler{notesService: notesService})
}

func (h *NotesHandler) CreateNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Method(ctx, "notes.Create")
	log.Debug(ctx, "handling create note request")
	req, err := bindJSON[CreateNoteRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequestBody.Error(), zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidRequestBody.Error())
	}
	note, err := h.notesService.CreateNote(ctx, req.Title, req.Content)
	if err != nil {
		log.Error(ctx, "failed to create note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusCreated, NoteResponse{Note: noteToAPI(note)})
}

func (h *NotesHandler) GetNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Method(ctx, "notes.Get")
	log.Debug(ctx, "handling get note request")
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrInvalidNoteID.Error())
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidNoteID.Error())
	}
	note, err := h.notesService.GetNote(ctx, noteID)
	if err != nil {
		log.Error(ctx, "failed to get note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusOK, noteToAPI(note))
}

func (h *NotesHandler) ListNotes(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Method(ctx, "notes.List")
	log.Debug(ctx, "handling list notes request")
	limit := queryInt(c, "limit", 10)
	if limit <= 0 {
		log.Error(ctx, domain.ErrInvalidPagination.Error())
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidPagination.Error())
	}
	offset := queryInt(c, "offset", 0)
	if offset < 0 {
		log.Error(ctx, domain.ErrInvalidPagination.Error())
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidPagination.Error())
	}
	limit32 := int32(math.MaxInt32)
	if limit >= 0 && limit <= math.MaxInt32 {
		limit32 = int32(limit)
	}
	offset32 := int32(math.MaxInt32)
	if offset >= 0 && offset <= math.MaxInt32 {
		offset32 = int32(offset)
	}
	notes, total, err := h.notesService.ListNotes(ctx, limit32, offset32)
	if err != nil {
		log.Error(ctx, "failed to list notes", zap.Error(err))
		return handleError(c, err)
	}
	total32 := int32(math.MaxInt32)
	if total >= 0 && total <= math.MaxInt32 {
		total32 = int32(total)
	}
	body := ListNotesResponse{
		Notes:      make([]*Note, len(notes)),
		TotalCount: total32,
		Offset:     offset32,
		Limit:      limit32,
	}
	for i, n := range notes {
		body.Notes[i] = noteToAPI(n)
	}
	return jsonResponse(c, fiber.StatusOK, body)
}

func (h *NotesHandler) UpdateNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Method(ctx, "notes.Update")
	log.Debug(ctx, "handling update note request")
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrInvalidNoteID.Error())
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidNoteID.Error())
	}
	req, err := bindJSON[UpdateNoteRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrInvalidRequestBody.Error(), zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidRequestBody.Error())
	}
	note, err := h.notesService.UpdateNote(ctx, noteID, req.Title, req.Content)
	if err != nil {
		log.Error(ctx, "failed to update note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusOK, NoteResponse{Note: new(Note{
		ID:        note.ID,
		UserID:    note.UserID,
		Title:     note.Title,
		Content:   note.Content,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
	})})
}

func (h *NotesHandler) DeleteNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Method(ctx, "notes.Delete")
	log.Debug(ctx, "handling delete note request")
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrInvalidNoteID.Error())
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrInvalidNoteID.Error())
	}
	if err := h.notesService.DeleteNote(ctx, noteID); err != nil {
		log.Error(ctx, "failed to delete note", zap.Error(err))
		return handleError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
