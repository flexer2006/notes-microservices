package http

import (
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
	log := logger.Log(ctx).With(zap.String("handler", "notes.Create"))
	log.Debug(ctx, domain.LogHandlerCreateNote)
	req, err := bindJSON[CreateNoteRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrMsgInvalidRequestBody, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidRequestBody)
	}
	note, err := h.notesService.CreateNote(ctx, req.Title, req.Content)
	if err != nil {
		log.Error(ctx, "failed to create note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusCreated, NoteResponse{Note: new(Note{ID: note.ID, UserID: note.UserID, Title: note.Title, Content: note.Content, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt})})
}

func (h *NotesHandler) GetNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "notes.Get"))
	log.Debug(ctx, domain.LogHandlerGetNote)
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrMsgInvalidNoteID)
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidNoteID)
	}
	note, err := h.notesService.GetNote(ctx, noteID)
	if err != nil {
		log.Error(ctx, "failed to get note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusOK, note)
}

func (h *NotesHandler) ListNotes(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "notes.List"))
	log.Debug(ctx, domain.LogHandlerListNotes)
	limit := queryInt(c, "limit", 10)
	if limit <= 0 {
		log.Error(ctx, domain.ErrMsgInvalidPagination)
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidPagination)
	}
	offset := queryInt(c, "offset", 0)
	if offset < 0 {
		log.Error(ctx, domain.ErrMsgInvalidPagination)
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidPagination)
	}
	notes, total, err := h.notesService.ListNotes(ctx, int32(limit), int32(offset))
	if err != nil {
		log.Error(ctx, "failed to list notes", zap.Error(err))
		return handleError(c, err)
	}
	body := ListNotesResponse{Notes: make([]*Note, len(notes)), TotalCount: int32(total), Offset: int32(offset), Limit: int32(limit)}
	for i, n := range notes {
		body.Notes[i] = new(Note{
			ID:        n.ID,
			UserID:    n.UserID,
			Title:     n.Title,
			Content:   n.Content,
			CreatedAt: n.CreatedAt,
			UpdatedAt: n.UpdatedAt,
		})
	}
	return jsonResponse(c, fiber.StatusOK, body)
}

func (h *NotesHandler) UpdateNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "notes.Update"))
	log.Debug(ctx, domain.LogHandlerUpdateNote)
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrMsgInvalidNoteID)
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidNoteID)
	}
	req, err := bindJSON[UpdateNoteRequest](c)
	if err != nil {
		log.Error(ctx, domain.ErrMsgInvalidRequestBody, zap.Error(err))
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidRequestBody)
	}
	note, err := h.notesService.UpdateNote(ctx, noteID, req.Title, req.Content)
	if err != nil {
		log.Error(ctx, "failed to update note", zap.Error(err))
		return handleError(c, err)
	}
	return jsonResponse(c, fiber.StatusOK, NoteResponse{Note: new(Note{ID: note.ID, UserID: note.UserID, Title: note.Title, Content: note.Content, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt})})
}

func (h *NotesHandler) DeleteNote(c fiber.Ctx) error {
	ctx := userCtx(c)
	log := logger.Log(ctx).With(zap.String("handler", "notes.Delete"))
	log.Debug(ctx, domain.LogHandlerDeleteNote)
	noteID := c.Params("note_id")
	if noteID == "" {
		log.Error(ctx, domain.ErrMsgInvalidNoteID)
		return errorResponse(c, fiber.StatusBadRequest, domain.ErrMsgInvalidNoteID)
	}
	if err := h.notesService.DeleteNote(ctx, noteID); err != nil {
		log.Error(ctx, "failed to delete note", zap.Error(err))
		return handleError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
