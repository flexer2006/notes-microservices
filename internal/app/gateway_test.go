package app_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/flexer2006/notes-microservices/internal/app"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func samplePair() *domain.TokenPair {
	return new(domain.TokenPair{
		UserID: "u1", Username: testUser, AccessToken: "a", RefreshToken: "r",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
}

func sampleNote() *domain.Note {
	return new(domain.Note{ID: "n1", UserID: "u1", Title: "t", Content: "c"})
}

func accessJWT(t *testing.T, ttl time.Duration) string {
	t.Helper()

	now := time.Now().UTC()
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.RegisteredClaims{
		ExpiresAt: jwtlib.NewNumericDate(now.Add(ttl)),
		IssuedAt:  jwtlib.NewNumericDate(now),
	})

	s, err := tok.SignedString([]byte("test-secret-key-32-bytes-minimum!!"))
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestGatewayAuth(t *testing.T) {
	t.Parallel()

	pair := samplePair()
	backend := new(stubAuthBackend{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return pair, nil
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return pair, nil
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return pair, nil
		},
		logout: func(context.Context, string) error { return nil },
	})
	svc := app.NewAuthService(backend, new(stubCache{}))

	got, err := svc.Register(context.Background(), testEmail, testUser, testPass)
	testkit.MyErrIs(t, err, nil)

	if got.UserID != "u1" {
		t.Fatalf("%+v", got)
	}

	_, err = svc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, nil)
	_, err = svc.RefreshTokens(context.Background(), "r")
	testkit.MyErrIs(t, err, nil)
	testkit.MyErrIs(t, svc.Logout(context.Background(), "r"), nil)

	fail := new(stubAuthBackend{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return nil, errBoom
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return nil, errBoom
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return nil, errBoom
		},
		logout: func(context.Context, string) error { return errBoom },
	})
	svc = app.NewAuthService(fail, new(stubCache{}))
	_, err = svc.Register(context.Background(), testEmail, testUser, testPass)
	testkit.MyErrIs(t, err, errBoom)
	_, err = svc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, errBoom)
	_, err = svc.RefreshTokens(context.Background(), "r")
	testkit.MyErrIs(t, err, errBoom)
	testkit.MyErrIs(t, svc.Logout(context.Background(), "r"), errBoom)
}

func TestGatewayProfile(t *testing.T) {
	t.Parallel()

	user := sampleUser()

	raw, err := json.Marshal(user)
	if err != nil {
		t.Fatal(err)
	}

	token := accessJWT(t, time.Hour)
	hits := 0
	cache := new(stubCache{})
	backend := new(stubAuthBackend{
		profile: func(context.Context, string) (*domain.User, error) {
			hits++

			return user, nil
		},
	})
	svc := app.NewAuthService(backend, cache)

	got, err := svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, nil)

	if got.ID != "u1" || hits != 1 {
		t.Fatalf("got=%+v hits=%d", got, hits)
	}

	cache.get = func(_ context.Context, key string) (string, error) {
		switch {
		case len(key) >= 12 && key[:12] == "profile:tok:":
			return "u1", nil
		case key == "profile:user:u1":
			return string(raw), nil
		default:
			return "", errMiss
		}
	}

	_, err = svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, nil)

	if hits != 1 {
		t.Fatalf("cache bypass hits=%d", hits)
	}

	cache.get = func(_ context.Context, key string) (string, error) {
		if len(key) >= 12 && key[:12] == "profile:tok:" {
			return "u1", nil
		}

		return "", nil
	}

	_, err = svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, nil)

	if hits != 2 {
		t.Fatalf("empty profile cache hits=%d", hits)
	}

	cache.get = func(_ context.Context, key string) (string, error) {
		if len(key) >= 12 && key[:12] == "profile:tok:" {
			return "u1", nil
		}

		return "{bad", nil
	}

	_, err = svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, nil)

	if hits != 3 {
		t.Fatalf("bad json hits=%d", hits)
	}

	svc = app.NewAuthService(backend, new(stubCache{}))
	_, err = svc.GetUserProfile(context.Background(), "not-jwt")
	testkit.MyErrIs(t, err, nil)
	_, err = svc.GetUserProfile(context.Background(), accessJWT(t, -time.Hour))
	testkit.MyErrIs(t, err, nil)

	svc = app.NewAuthService(new(stubAuthBackend{
		profile: func(context.Context, string) (*domain.User, error) { return nil, errBoom },
	}), new(stubCache{}))
	_, err = svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, errBoom)

	svc = app.NewAuthService(backend, new(stubCache{
		set: func(context.Context, string, string, time.Duration) error { return errMiss },
	}))
	_, err = svc.GetUserProfile(context.Background(), token)
	testkit.MyErrIs(t, err, nil)
}

func TestGatewayNotes(t *testing.T) {
	t.Parallel()

	note := sampleNote()
	backend := new(stubNotesBackend{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return note, nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) { return note, nil },
		list: func(_ context.Context, _ string, limit, offset int) ([]*domain.Note, int, error) {
			if limit < 0 || offset < 0 {
				t.Fatalf("unclamped limit=%d offset=%d", limit, offset)
			}

			return []*domain.Note{note}, 1, nil
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return note, nil
		},
		delete: func(context.Context, string, string) error { return nil },
	})
	svc := app.NewNotesService(backend, new(stubCache{}))

	_, err := svc.CreateNote(context.Background(), "tok", "t", "c")
	testkit.MyErrIs(t, err, nil)
	_, err = svc.GetNote(context.Background(), "tok", "n1")
	testkit.MyErrIs(t, err, nil)

	list, total, err := svc.ListNotes(context.Background(), "tok", -5, -1)
	testkit.MyErrIs(t, err, nil)

	if total != 1 || len(list) != 1 {
		t.Fatalf("list=%d total=%d", len(list), total)
	}

	title := "x"
	_, err = svc.UpdateNote(context.Background(), "tok", "n1", new(title), nil)
	testkit.MyErrIs(t, err, nil)
	testkit.MyErrIs(t, svc.DeleteNote(context.Background(), "tok", "n1"), nil)

	var gotLimit int

	svc = app.NewNotesService(new(stubNotesBackend{
		list: func(_ context.Context, _ string, limit, _ int) ([]*domain.Note, int, error) {
			gotLimit = limit

			return nil, 0, nil
		},
	}), new(stubCache{}))
	_, _, err = svc.ListNotes(context.Background(), "tok", math.MaxInt32+1, 0)
	testkit.MyErrIs(t, err, nil)

	if gotLimit != math.MaxInt32 {
		t.Fatalf("limit=%d", gotLimit)
	}

	fail := new(stubNotesBackend{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return nil, errBoom
		},
		get: func(context.Context, string, string) (*domain.Note, error) { return nil, errBoom },
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return nil, 0, errBoom
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return nil, errBoom
		},
		delete: func(context.Context, string, string) error { return errBoom },
	})
	svc = app.NewNotesService(fail, new(stubCache{}))
	_, err = svc.CreateNote(context.Background(), "t", "a", "b")
	testkit.MyErrIs(t, err, errBoom)
	_, err = svc.GetNote(context.Background(), "t", "n")
	testkit.MyErrIs(t, err, errBoom)
	_, _, err = svc.ListNotes(context.Background(), "t", 1, 0)
	testkit.MyErrIs(t, err, errBoom)
	_, err = svc.UpdateNote(context.Background(), "t", "n", nil, nil)
	testkit.MyErrIs(t, err, errBoom)
	testkit.MyErrIs(t, svc.DeleteNote(context.Background(), "t", "n"), errBoom)
}

func TestGatewayInfra(t *testing.T) {
	t.Parallel()

	for _, want := range []error{domain.ErrServiceUnavailable, context.DeadlineExceeded} {
		svc := app.NewAuthService(new(stubAuthBackend{
			register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
				return nil, want
			},
		}), new(stubCache{}))
		_, err := svc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, want)
	}
}
