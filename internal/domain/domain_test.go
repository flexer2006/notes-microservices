package domain_test

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func TestValidateEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
		want  error
	}{
		{name: "ok", email: "a@b.co", want: nil},
		{name: "plus", email: "a+x@b.co", want: nil},
		{name: "empty", email: "", want: domain.ErrInvalidEmail},
		{name: "no_at", email: "ab.co", want: domain.ErrInvalidEmail},
		{name: "no_tld", email: "a@b", want: domain.ErrInvalidEmail},
		{name: "spaces", email: "a @b.co", want: domain.ErrInvalidEmail},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.MyErrIs(t, domain.ValidateEmail(tc.email), tc.want)
		})
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pass string
		want error
	}{
		{name: "ok", pass: "password1", want: nil},
		{name: "short", pass: "a1b2c3", want: domain.ErrPasswordTooShort},
		{name: "no_digit", pass: "password", want: domain.ErrPasswordTooWeak},
		{name: "no_letter", pass: "12345678", want: domain.ErrPasswordTooWeak},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.MyErrIs(t, domain.ValidatePassword(tc.pass), tc.want)
		})
	}
}

func TestNewUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		email    string
		username string
		want     error
	}{
		{name: "ok", email: "u@ex.com", username: "alice", want: nil},
		{name: "bad_email", email: "bad", username: "alice", want: domain.ErrInvalidEmail},
		{name: "empty_user", email: "u@ex.com", username: "  ", want: domain.ErrEmptyUsername},
		{
			name: "username_too_long", email: "u@ex.com",
			username: strings.Repeat("a", 51), want: domain.ErrUsernameTooLong,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			user, err := domain.NewUser(tc.email, tc.username, "hash")
			testkit.MyErrIs(t, err, tc.want)

			if tc.want != nil {
				if user != nil {
					t.Fatalf("user = %#v, want nil", user)
				}

				return
			}

			if user.Email != tc.email {
				t.Fatalf("email = %q, want %q", user.Email, tc.email)
			}

			if user.Username != tc.username {
				t.Fatalf("username = %q, want %q", user.Username, tc.username)
			}

			if user.PasswordHash != "hash" {
				t.Fatalf("password hash = %q", user.PasswordHash)
			}
		})
	}
}

func TestNewNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		userID  string
		title   string
		content string
		want    error
	}{
		{name: "ok", userID: "u1", title: "t", content: "c", want: nil},
		{name: "empty_user", userID: "", title: "t", content: "c", want: domain.ErrEmptyUserID},
		{
			name:    "empty_title",
			userID:  "u1",
			title:   " \t",
			content: "c",
			want:    domain.ErrEmptyNoteTitle,
		},
		{
			name: "title_too_long", userID: "u1",
			title: strings.Repeat("t", 256), content: "c", want: domain.ErrNoteTitleTooLong,
		},
		{
			name: "content_too_large", userID: "u1", title: "t",
			content: strings.Repeat("c", (64<<10)+1), want: domain.ErrNoteContentTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.want != nil {
				note, err := domain.NewNote(tc.userID, tc.title, tc.content)
				testkit.MyErrIs(t, err, tc.want)

				if note != nil {
					t.Fatalf("note = %#v, want nil", note)
				}

				return
			}

			synctest.Test(t, func(t *testing.T) {
				note, err := domain.NewNote(tc.userID, tc.title, tc.content)
				testkit.MyErrIs(t, err, nil)

				want := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
				if !note.CreatedAt.Equal(want) || !note.UpdatedAt.Equal(want) {
					t.Fatalf("timestamps = %v/%v, want %v", note.CreatedAt, note.UpdatedAt, want)
				}

				if note.UserID != tc.userID || note.Title != tc.title ||
					note.Content != tc.content {
					t.Fatalf("note = %#v", note)
				}
			})
		})
	}
}

func TestNoteApplyUpdate(t *testing.T) {
	t.Parallel()

	newTitle := "n"
	newContent := "x"
	blankTitle := " "
	longTitle := strings.Repeat("t", 256)
	hugeContent := strings.Repeat("c", (64<<10)+1)

	tests := []struct {
		name    string
		title   *string
		content *string
		want    error
		wantT   string
		wantC   string
	}{
		{name: "title", title: new(newTitle), content: nil, want: nil, wantT: "n", wantC: "c"},
		{name: "content", title: nil, content: new(newContent), want: nil, wantT: "t", wantC: "x"},
		{
			name:    "both",
			title:   new(newTitle),
			content: new(newContent),
			want:    nil,
			wantT:   "n",
			wantC:   "x",
		},
		{
			name:    "empty_title",
			title:   new(blankTitle),
			content: nil,
			want:    domain.ErrEmptyNoteTitle,
			wantT:   "t",
			wantC:   "c",
		},
		{
			name: "title_too_long", title: new(longTitle), content: nil,
			want: domain.ErrNoteTitleTooLong, wantT: "t", wantC: "c",
		},
		{
			name: "content_too_large", title: nil, content: new(hugeContent),
			want: domain.ErrNoteContentTooLarge, wantT: "t", wantC: "c",
		},
		{name: "noop_ptrs", title: nil, content: nil, want: nil, wantT: "t", wantC: "c"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				note, err := domain.NewNote("u1", "t", "c")
				testkit.MyErrIs(t, err, nil)

				created := note.CreatedAt

				time.Sleep(time.Second)

				err = note.ApplyUpdate(tc.title, tc.content)
				testkit.MyErrIs(t, err, tc.want)

				if note.Title != tc.wantT || note.Content != tc.wantC {
					t.Fatalf("note = %#v", note)
				}

				if tc.want != nil {
					if !note.UpdatedAt.Equal(created) {
						t.Fatalf("UpdatedAt changed on error: %v", note.UpdatedAt)
					}

					return
				}

				wantUpdated := created.Add(time.Second)
				if !note.UpdatedAt.Equal(wantUpdated) {
					t.Fatalf("UpdatedAt = %v, want %v", note.UpdatedAt, wantUpdated)
				}
			})
		})
	}
}

func FuzzValidateEmail(f *testing.F) {
	f.Add("a@b.co")
	f.Add("")
	f.Add("not-an-email")
	f.Fuzz(func(t *testing.T, email string) {
		err := domain.ValidateEmail(email)
		if err != nil && !errors.Is(err, domain.ErrInvalidEmail) {
			t.Fatalf("unexpected err %v", err)
		}
	})
}

func FuzzValidatePassword(f *testing.F) {
	f.Add("password1")
	f.Add("short1")
	f.Add("nodigits")
	f.Fuzz(func(t *testing.T, pass string) {
		err := domain.ValidatePassword(pass)
		switch {
		case err == nil:
			if len(pass) < 8 {
				t.Fatalf("accepted short password")
			}
		case errors.Is(err, domain.ErrPasswordTooShort), errors.Is(err, domain.ErrPasswordTooWeak):
		default:
			t.Fatalf("unexpected err %v", err)
		}
	})
}
