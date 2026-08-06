package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	httpadapter "github.com/flexer2006/notes-microservices/internal/adapters/http"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

var errBoom = errors.New("boom")

type stubAuth struct {
	register func(context.Context, string, string, string) (*domain.TokenPair, error)
	login    func(context.Context, string, string) (*domain.TokenPair, error)
	refresh  func(context.Context, string) (*domain.TokenPair, error)
	logout   func(context.Context, string) error
	profile  func(context.Context, string) (*domain.User, error)
}

func (s *stubAuth) Register(
	ctx context.Context,
	email, user, pass string,
) (*domain.TokenPair, error) {
	return s.register(ctx, email, user, pass)
}

func (s *stubAuth) Login(ctx context.Context, email, pass string) (*domain.TokenPair, error) {
	return s.login(ctx, email, pass)
}

func (s *stubAuth) RefreshTokens(ctx context.Context, tok string) (*domain.TokenPair, error) {
	return s.refresh(ctx, tok)
}

func (s *stubAuth) Logout(ctx context.Context, tok string) error { return s.logout(ctx, tok) }

func (s *stubAuth) GetUserProfile(ctx context.Context, tok string) (*domain.User, error) {
	return s.profile(ctx, tok)
}

type stubNotes struct {
	create func(context.Context, string, string, string) (*domain.Note, error)
	get    func(context.Context, string, string) (*domain.Note, error)
	list   func(context.Context, string, int, int) ([]*domain.Note, int, error)
	update func(context.Context, string, string, *string, *string) (*domain.Note, error)
	delete func(context.Context, string, string) error
}

func (s *stubNotes) CreateNote(
	ctx context.Context,
	tok, title, content string,
) (*domain.Note, error) {
	return s.create(ctx, tok, title, content)
}

func (s *stubNotes) GetNote(ctx context.Context, tok, id string) (*domain.Note, error) {
	return s.get(ctx, tok, id)
}

func (s *stubNotes) ListNotes(
	ctx context.Context,
	tok string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	return s.list(ctx, tok, limit, offset)
}

func (s *stubNotes) UpdateNote(
	ctx context.Context,
	tok, id string,
	title, content *string,
) (*domain.Note, error) {
	return s.update(ctx, tok, id, title, content)
}

func (s *stubNotes) DeleteNote(ctx context.Context, tok, id string) error {
	return s.delete(ctx, tok, id)
}

type stubCache struct {
	ping error
}

func (s *stubCache) Set(context.Context, string, string, time.Duration) error { return nil }

func (s *stubCache) Get(context.Context, string) (string, error) { return "", errBoom }

func (s *stubCache) Delete(context.Context, string) error { return nil }

func (s *stubCache) Ping(context.Context) error { return s.ping }

func (s *stubCache) Close() error { return nil }

func pair() *domain.TokenPair {
	return new(domain.TokenPair{
		UserID: "u1", Username: "alice", AccessToken: "a", RefreshToken: "r",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
}

func note() *domain.Note {
	return new(domain.Note{ID: "n1", UserID: "u1", Title: "t", Content: "c"})
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string, hdr map[string]string) (
	int,
	map[string]any,
) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(),
		method,
		path,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Errorf("close response body: %v", closeErr)
		}
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]any{}
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &out)
	}

	return resp.StatusCode, out
}

func TestHealthReadyAndRoutes(t *testing.T) {
	t.Parallel()

	auth := new(stubAuth{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		logout: func(context.Context, string) error { return nil },
		profile: func(context.Context, string) (*domain.User, error) {
			return new(domain.User{ID: "u1", Email: "a@ex.com", Username: "alice"}), nil
		},
	})
	notes := new(stubNotes{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return note(), nil
		},
		get: func(context.Context, string, string) (*domain.Note, error) { return note(), nil },
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			return []*domain.Note{note()}, 1, nil
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return note(), nil
		},
		delete: func(context.Context, string, string) error { return nil },
	})
	cache := new(stubCache{})

	app := fiber.New()
	httpadapter.SetupRouter(app, auth, notes, cache)

	code, body := doJSON(t, app, http.MethodGet, "/health", "", nil)
	if code != fiber.StatusOK || body["status"] != "ok" {
		t.Fatalf("health %d %v", code, body)
	}

	code, body = doJSON(t, app, http.MethodGet, "/ready", "", nil)
	if code != fiber.StatusOK || body["status"] != "ready" {
		t.Fatalf("ready %d %v", code, body)
	}

	cache.ping = errBoom

	code, body = doJSON(t, app, http.MethodGet, "/ready", "", nil)
	if code != fiber.StatusServiceUnavailable {
		t.Fatalf("ready fail %d %v", code, body)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/missing", "", nil)
	if code != fiber.StatusNotFound {
		t.Fatalf("404 got %d", code)
	}

	authHdr := map[string]string{"Authorization": "Bearer tok"}

	code, _ = doJSON(t, app, http.MethodPost, "/api/v1/auth/register",
		`{"email":"a@ex.com","username":"alice","password":"password1"}`, nil)
	if code != fiber.StatusCreated {
		t.Fatalf("register %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/api/v1/auth/login",
		`{"email":"a@ex.com","password":"password1"}`, nil)
	if code != fiber.StatusOK {
		t.Fatalf("login %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"r"}`, nil)
	if code != fiber.StatusOK {
		t.Fatalf("refresh %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/api/v1/auth/logout",
		`{"refresh_token":"r"}`, nil)
	if code != fiber.StatusOK {
		t.Fatalf("logout %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/api/v1/user/profile", "", authHdr)
	if code != fiber.StatusOK {
		t.Fatalf("profile %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/api/v1/notes/",
		`{"title":"t","content":"c"}`, authHdr)
	if code != fiber.StatusCreated {
		t.Fatalf("create note %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/api/v1/notes/n1", "", authHdr)
	if code != fiber.StatusOK {
		t.Fatalf("get note %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/api/v1/notes/?limit=5&offset=0", "", authHdr)
	if code != fiber.StatusOK {
		t.Fatalf("list %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPatch, "/api/v1/notes/n1",
		`{"title":"x"}`, authHdr)
	if code != fiber.StatusOK {
		t.Fatalf("patch %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPut, "/api/v1/notes/n1",
		`{"title":"t","content":"c"}`, authHdr)
	if code != fiber.StatusOK {
		t.Fatalf("put %d", code)
	}

	code, _ = doJSON(t, app, http.MethodDelete, "/api/v1/notes/n1", "", authHdr)
	if code != fiber.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
}

func TestAuthValidationAndErrors(t *testing.T) {
	t.Parallel()

	auth := new(stubAuth{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return nil, domain.ErrEmailAlreadyExists
		},
		login: func(context.Context, string, string) (*domain.TokenPair, error) {
			return nil, domain.ErrInvalidCredentials
		},
		refresh: func(context.Context, string) (*domain.TokenPair, error) {
			return nil, domain.ErrInvalidRefreshToken
		},
		logout: func(context.Context, string) error { return fault.ErrCircuitOpen },
		profile: func(context.Context, string) (*domain.User, error) {
			return nil, domain.ErrUnauthorized
		},
	})
	app := fiber.New()
	h := httpadapter.NewAuthHandler(auth)
	app.Post("/register", h.Register)
	app.Post("/login", h.Login)
	app.Post("/refresh", h.RefreshTokens)
	app.Post("/logout", h.Logout)
	app.Get("/profile", httpadapter.NewAuthMiddleware(), h.GetProfile)

	code, _ := doJSON(t, app, http.MethodPost, "/register", `{`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("bad json %d", code)
	}

	code, _ = doJSON(
		t,
		app,
		http.MethodPost,
		"/register",
		`{"email":"","username":"","password":""}`,
		nil,
	)
	if code != fiber.StatusBadRequest {
		t.Fatalf("empty fields %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/register",
		`{"email":"bad","username":"alice","password":"password1"}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("bad email %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/register",
		`{"email":"a@ex.com","username":"alice","password":"short"}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("weak pass %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/register",
		`{"email":"a@ex.com","username":"alice","password":"password1"}`, nil)
	if code != fiber.StatusConflict {
		t.Fatalf("exists %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/login", `{"email":"","password":""}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("login empty %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/login",
		`{"email":"bad","password":"x"}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("login email %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/login", `{`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("login json %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/login",
		`{"email":"a@ex.com","password":"password1"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("login fail %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/refresh", `{`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("refresh json %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/refresh", `{"refresh_token":""}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("refresh empty %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/refresh", `{"refresh_token":"r"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("refresh fail %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/logout", `{`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("logout json %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/logout", `{"refresh_token":""}`, nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("logout empty %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/logout", `{"refresh_token":"r"}`, nil)
	if code != fiber.StatusServiceUnavailable {
		t.Fatalf("logout cb %d", code)
	}

	bare := fiber.New()
	bare.Get("/profile", httpadapter.NewAuthHandler(auth).GetProfile)

	code, _ = doJSON(t, bare, http.MethodGet, "/profile", "", nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("profile bare %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/profile", "", nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("no auth %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/profile", "", map[string]string{
		"Authorization": "Basic x",
	})
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bad scheme %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/profile", "", map[string]string{
		"Authorization": "Bearer tok",
	})
	if code != fiber.StatusUnauthorized {
		t.Fatalf("profile fail %d", code)
	}
}

func TestNotesErrorsAndQuery(t *testing.T) {
	t.Parallel()

	notes := new(stubNotes{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			return nil, domain.ErrEmptyNoteTitle
		},
		get: func(context.Context, string, string) (*domain.Note, error) {
			return nil, domain.ErrNoteNotFoundOrNotOwned
		},
		list: func(_ context.Context, _ string, limit, offset int) ([]*domain.Note, int, error) {
			if limit != 10 || offset != 0 {
				return nil, 0, errBoom
			}

			return nil, 0, errBoom
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			return nil, domain.ErrNoteNotFound
		},
		delete: func(context.Context, string, string) error { return domain.ErrUserNotFound },
	})
	app := fiber.New()
	handler := httpadapter.NewNotesHandler(notes)

	app.Use(httpadapter.NewAuthMiddleware())
	app.Post("/", handler.CreateNote)
	app.Get("/:note_id", handler.GetNote)
	app.Get("/", handler.ListNotes)
	app.Patch("/:note_id", handler.UpdateNote)
	app.Put("/:note_id", handler.ReplaceNote)
	app.Delete("/:note_id", handler.DeleteNote)

	hdr := map[string]string{"Authorization": "Bearer tok"}

	code, _ := doJSON(t, app, http.MethodPost, "/", `{"title":"t","content":"c"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("create no auth %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/", `{"title":"  ","content":"c"}`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("empty title %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/", `{`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("bad body %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPost, "/", `{"title":"t","content":"c"}`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("create svc %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/n1", "", hdr)
	if code != fiber.StatusNotFound {
		t.Fatalf("get %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/", "", hdr)
	if code != fiber.StatusInternalServerError {
		t.Fatalf("list defaults %d", code)
	}

	code, _ = doJSON(t, app, http.MethodGet, "/?limit=bad&offset=-1", "", hdr)
	if code != fiber.StatusInternalServerError {
		t.Fatalf("list boom %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPatch, "/n1", `{`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("patch body %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPatch, "/n1", `{"title":"x"}`, hdr)
	if code != fiber.StatusNotFound {
		t.Fatalf("patch %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPut, "/n1", `{`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("put body %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPut, "/n1", `{"title":"t"}`, hdr)
	if code != fiber.StatusBadRequest {
		t.Fatalf("put incomplete %d", code)
	}

	code, _ = doJSON(t, app, http.MethodPut, "/n1", `{"title":"t","content":"c"}`, hdr)
	if code != fiber.StatusNotFound {
		t.Fatalf("put %d", code)
	}

	code, _ = doJSON(t, app, http.MethodDelete, "/n1", "", hdr)
	if code != fiber.StatusNotFound {
		t.Fatalf("delete %d", code)
	}

	bare := fiber.New()
	bareHandler := httpadapter.NewNotesHandler(notes)
	bare.Post("/", bareHandler.CreateNote)
	bare.Get("/", bareHandler.ListNotes)
	bare.Get("/:note_id", bareHandler.GetNote)
	bare.Patch("/:note_id", bareHandler.UpdateNote)
	bare.Put("/:note_id", bareHandler.ReplaceNote)
	bare.Delete("/:note_id", bareHandler.DeleteNote)

	code, _ = doJSON(t, bare, http.MethodPost, "/", `{"title":"t","content":"c"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare create %d", code)
	}

	code, _ = doJSON(t, bare, http.MethodGet, "/", "", nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare list %d", code)
	}

	code, _ = doJSON(t, bare, http.MethodGet, "/n1", "", nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare get %d", code)
	}

	code, _ = doJSON(t, bare, http.MethodPatch, "/n1", `{"title":"x"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare patch %d", code)
	}

	code, _ = doJSON(t, bare, http.MethodPut, "/n1", `{"title":"t","content":"c"}`, nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare put %d", code)
	}

	code, _ = doJSON(t, bare, http.MethodDelete, "/n1", "", nil)
	if code != fiber.StatusUnauthorized {
		t.Fatalf("bare delete %d", code)
	}
}

func TestRateLimitReached(t *testing.T) {
	t.Parallel()

	auth := new(stubAuth{
		register: func(context.Context, string, string, string) (*domain.TokenPair, error) {
			return pair(), nil
		},
		login:   func(context.Context, string, string) (*domain.TokenPair, error) { panic("unexpected") },
		refresh: func(context.Context, string) (*domain.TokenPair, error) { panic("unexpected") },
		logout:  func(context.Context, string) error { panic("unexpected") },
		profile: func(context.Context, string) (*domain.User, error) { panic("unexpected") },
	})
	notes := new(stubNotes{
		create: func(context.Context, string, string, string) (*domain.Note, error) {
			panic("unexpected")
		},
		get: func(context.Context, string, string) (*domain.Note, error) {
			panic("unexpected")
		},
		list: func(context.Context, string, int, int) ([]*domain.Note, int, error) {
			panic("unexpected")
		},
		update: func(context.Context, string, string, *string, *string) (*domain.Note, error) {
			panic("unexpected")
		},
		delete: func(context.Context, string, string) error { panic("unexpected") },
	})

	app := fiber.New()
	httpadapter.SetupRouter(app, auth, notes, new(stubCache{}))

	body := `{"email":"a@ex.com","username":"alice","password":"password1"}`

	var last int

	for range 6 {
		last, _ = doJSON(t, app, http.MethodPost, "/api/v1/auth/register", body, nil)
	}

	if last != fiber.StatusTooManyRequests {
		t.Fatalf("rate limit got %d", last)
	}
}

func TestRecoveryAndRequestID(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(httpadapter.NewRequestIDMiddleware())
	app.Use(httpadapter.NewRecoveryMiddleware())
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/panic", func(fiber.Ctx) error {
		panic("boom")
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/ok", nil)
	req.Header.Set("X-Request-ID", "fixed-id")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	closeErr := resp.Body.Close()
	if closeErr != nil {
		t.Errorf("close response body: %v", closeErr)
	}

	if resp.Header.Get("X-Request-ID") != "fixed-id" {
		t.Fatalf("rid=%q", resp.Header.Get("X-Request-ID"))
	}

	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "/panic", nil)

	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	closeErr = resp.Body.Close()
	if closeErr != nil {
		t.Errorf("close response body: %v", closeErr)
	}

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("panic status %d", resp.StatusCode)
	}
}

func TestLoggerMiddlewareError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(httpadapter.NewLoggerMiddleware())
	app.Get("/fail", func(fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "nope")
	})

	code, body := doJSON(t, app, http.MethodGet, "/fail", "", nil)
	if code != fiber.StatusBadRequest {
		t.Fatalf("code=%d body=%v", code, body)
	}
}
