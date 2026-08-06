package app_test

import (
	"context"
	"testing"

	"github.com/flexer2006/notes-microservices/internal/app"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func TestNotes(t *testing.T) {
	t.Parallel()

	note := new(domain.Note{ID: "n1", UserID: "u1", Title: "t", Content: "c"})
	repo := new(stubNotes{
		create: func(_ context.Context, n *domain.Note) (*domain.Note, error) {
			n.ID = "n1"

			return n, nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) { return note, nil },
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return []*domain.Note{note}, 1, nil
		},
		update: func(context.Context, *domain.Note) error { return nil },
		delete: func(context.Context, string, string) error { return nil },
	})
	uc := app.NewNoteUseCase(repo)

	created, err := uc.CreateNote(context.Background(), "u1", "t", "c")
	testkit.MyErrIs(t, err, nil)

	if created.ID != "n1" {
		t.Fatalf("%+v", created)
	}

	_, err = uc.GetNote(context.Background(), "u1", "n1")
	testkit.MyErrIs(t, err, nil)

	title, content := "new", "body"
	_, err = uc.UpdateNote(context.Background(), "u1", "n1", new(title), new(content))
	testkit.MyErrIs(t, err, nil)

	list, total, err := uc.ListNotes(context.Background(), "u1", 0, -1)
	testkit.MyErrIs(t, err, nil)

	if total != 1 || len(list) != 1 {
		t.Fatalf("list=%d total=%d", len(list), total)
	}

	testkit.MyErrIs(t, uc.DeleteNote(context.Background(), "u1", "n1"), nil)
}

func TestNotesErrors(t *testing.T) {
	t.Parallel()

	uc := app.NewNoteUseCase(new(stubNotes{}))
	_, err := uc.CreateNote(context.Background(), "", "t", "c")
	testkit.MyErrIs(t, err, domain.ErrEmptyUserID)
	_, err = uc.CreateNote(context.Background(), "u1", "  ", "c")
	testkit.MyErrIs(t, err, domain.ErrEmptyNoteTitle)

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			create: func(context.Context, *domain.Note) (*domain.Note, error) {
				return nil, errBoom
			},
		}))
		_, err := uc.CreateNote(context.Background(), "u1", "t", "c")
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("get", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			get: func(context.Context, string, string) (*domain.Note, error) {
				return nil, domain.ErrNoteNotFoundOrNotOwned
			},
		}))
		_, err := uc.GetNote(context.Background(), "u1", "n1")
		testkit.MyErrIs(t, err, domain.ErrNoteNotFoundOrNotOwned)
	})

	t.Run("list_clamp", func(t *testing.T) {
		t.Parallel()

		var gotLimit, gotOffset int

		uc := app.NewNoteUseCase(new(stubNotes{
			list: func(_ context.Context, _ string, limit, offset int) ([]*domain.Note, int, error) {
				gotLimit, gotOffset = limit, offset

				return nil, 0, nil
			},
		}))
		_, _, err := uc.ListNotes(context.Background(), "u1", 500, -3)
		testkit.MyErrIs(t, err, nil)

		if gotLimit != 100 || gotOffset != 0 {
			t.Fatalf("limit=%d offset=%d", gotLimit, gotOffset)
		}
	})

	t.Run("list_err", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
				return nil, 0, errBoom
			},
		}))
		_, _, err := uc.ListNotes(context.Background(), "u1", 10, 0)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("update_get", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			get: func(context.Context, string, string) (*domain.Note, error) {
				return nil, domain.ErrNoteNotFoundOrNotOwned
			},
		}))
		title := "x"
		_, err := uc.UpdateNote(context.Background(), "u1", "n1", new(title), nil)
		testkit.MyErrIs(t, err, domain.ErrNoteNotFoundOrNotOwned)
	})

	t.Run("update_apply", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			get: func(context.Context, string, string) (*domain.Note, error) {
				return new(domain.Note{ID: "n1", UserID: "u1", Title: "t"}), nil
			},
		}))
		empty := "  "
		_, err := uc.UpdateNote(context.Background(), "u1", "n1", new(empty), nil)
		testkit.MyErrIs(t, err, domain.ErrEmptyNoteTitle)
	})

	t.Run("update_repo", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			get: func(context.Context, string, string) (*domain.Note, error) {
				return new(domain.Note{ID: "n1", UserID: "u1", Title: "t"}), nil
			},
			update: func(context.Context, *domain.Note) error { return errBoom },
		}))
		title := "ok"
		_, err := uc.UpdateNote(context.Background(), "u1", "n1", new(title), nil)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		uc := app.NewNoteUseCase(new(stubNotes{
			delete: func(context.Context, string, string) error { return errBoom },
		}))
		testkit.MyErrIs(t, uc.DeleteNote(context.Background(), "u1", "n1"), errBoom)
	})
}
