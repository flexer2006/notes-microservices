package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

const defaultListLimit = 10

type NotesHandler struct {
	notesService ports.NotesService
}

type CreateNoteRequest struct {
	Title   string `json:"title"   validate:"required"`
	Content string `json:"content" validate:"required"`
}

type UpdateNoteRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

type Note struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
}

type NoteResponse struct {
	Note *Note `json:"note"`
}

type ListNotesResponse struct {
	Notes      []*Note `json:"notes"`
	TotalCount int     `json:"total_count"`
	Offset     int     `json:"offset"`
	Limit      int     `json:"limit"`
}

func NewNotesHandler(notesService ports.NotesService) *NotesHandler {
	return new(NotesHandler{notesService: notesService})
}

func (h *NotesHandler) CreateNote(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token := authctx.BearerTokenFrom(ctx)
	if token == "" {
		return errorResponse(fctx, fiber.StatusUnauthorized, errNoAuthHeader.Error())
	}

	req, err := bindJSON[CreateNoteRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if strings.TrimSpace(req.Title) == "" {
		return handleError(fctx, domain.ErrEmptyNoteTitle)
	}

	note, err := h.notesService.CreateNote(ctx, token, req.Title, req.Content)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusCreated, NoteResponse{Note: noteToAPI(note)})
}

func (h *NotesHandler) GetNote(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token, noteID, err := requireTokenAndNoteID(fctx)
	if err != nil {
		return handleError(fctx, err)
	}

	note, err := h.notesService.GetNote(ctx, token, noteID)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, NoteResponse{Note: noteToAPI(note)})
}

func (h *NotesHandler) ListNotes(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token := authctx.BearerTokenFrom(ctx)
	if token == "" {
		return errorResponse(fctx, fiber.StatusUnauthorized, errNoAuthHeader.Error())
	}

	limit := queryInt(fctx, "limit", defaultListLimit)
	offset := queryInt(fctx, "offset", 0)

	notes, total, err := h.notesService.ListNotes(ctx, token, limit, offset)
	if err != nil {
		return handleError(fctx, err)
	}

	body := ListNotesResponse{
		Notes:      make([]*Note, len(notes)),
		TotalCount: total,
		Offset:     offset,
		Limit:      limit,
	}
	for i, n := range notes {
		body.Notes[i] = noteToAPI(n)
	}

	return jsonResponse(fctx, fiber.StatusOK, body)
}

func (h *NotesHandler) UpdateNote(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token, noteID, err := requireTokenAndNoteID(fctx)
	if err != nil {
		return handleError(fctx, err)
	}

	req, err := bindJSON[UpdateNoteRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	note, err := h.notesService.UpdateNote(ctx, token, noteID, req.Title, req.Content)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, NoteResponse{Note: noteToAPI(note)})
}

func (h *NotesHandler) ReplaceNote(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token, noteID, err := requireTokenAndNoteID(fctx)
	if err != nil {
		return handleError(fctx, err)
	}

	req, err := bindJSON[UpdateNoteRequest](fctx)
	if err != nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Title == nil || req.Content == nil {
		return errorResponse(fctx, fiber.StatusBadRequest, "title and content are required")
	}

	note, err := h.notesService.UpdateNote(ctx, token, noteID, req.Title, req.Content)
	if err != nil {
		return handleError(fctx, err)
	}

	return jsonResponse(fctx, fiber.StatusOK, NoteResponse{Note: noteToAPI(note)})
}

func (h *NotesHandler) DeleteNote(fctx fiber.Ctx) error {
	ctx := userCtx(fctx)

	token, noteID, err := requireTokenAndNoteID(fctx)
	if err != nil {
		return handleError(fctx, err)
	}

	deleteErr := h.notesService.DeleteNote(ctx, token, noteID)
	if deleteErr != nil {
		return handleError(fctx, deleteErr)
	}

	sendErr := fctx.SendStatus(fiber.StatusNoContent)
	if sendErr != nil {
		return fmt.Errorf("write no-content status: %w", sendErr)
	}

	return nil
}

func requireTokenAndNoteID(fctx fiber.Ctx) (string, string, error) {
	token := authctx.BearerTokenFrom(userCtx(fctx))
	if token == "" {
		return "", "", fiber.NewError(fiber.StatusUnauthorized, errNoAuthHeader.Error())
	}

	noteID := fctx.Params("note_id")
	if noteID == "" {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "invalid note id")
	}

	return token, noteID, nil
}
