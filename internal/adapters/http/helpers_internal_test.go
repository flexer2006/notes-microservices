package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/flexer2006/notes-microservices/internal/authctx"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/fault"
)

var errOther = errors.New("other")

func TestHTTPErrorFromDomain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err    error
		status int
		ok     bool
	}{
		{err: nil, ok: false},
		{err: domain.ErrInvalidEmail, status: fiber.StatusBadRequest, ok: true},
		{err: domain.ErrUnauthorized, status: fiber.StatusUnauthorized, ok: true},
		{err: domain.ErrNoteNotFound, status: fiber.StatusNotFound, ok: true},
		{err: domain.ErrEmailAlreadyExists, status: fiber.StatusConflict, ok: true},
		{err: domain.ErrServiceUnavailable, status: fiber.StatusServiceUnavailable, ok: true},
		{err: fault.ErrCircuitOpen, status: fiber.StatusServiceUnavailable, ok: true},
		{err: errOther, status: fiber.StatusInternalServerError, ok: true},
	}

	for _, tc := range cases {
		status, _, ok := httpErrorFromDomain(tc.err)
		if ok != tc.ok || (ok && status != tc.status) {
			t.Fatalf("%v -> %d ok=%v", tc.err, status, ok)
		}
	}
}

func TestHandleErrorFiberAndQuery(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/fe", func(c fiber.Ctx) error {
		return handleError(c, fiber.NewError(fiber.StatusTeapot, "teapot"))
	})
	app.Get("/qi", func(c fiber.Ctx) error {
		n := queryInt(c, "n", 7)

		return c.JSON(fiber.Map{"n": n})
	})

	code, body := doReq(t, app, "/fe")
	if code != fiber.StatusTeapot || body["error"] != "teapot" {
		t.Fatalf("%d %v", code, body)
	}

	code, body = doReq(t, app, "/qi?n=3")

	n, ok := body["n"].(float64)
	if code != 200 || !ok || int(n) != 3 {
		t.Fatalf("%d %v", code, body)
	}
}

func TestRequireTokenAndNoteID(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/notes", func(c fiber.Ctx) error {
		c.SetContext(authctx.WithBearerToken(c.Context(), "tok"))

		_, _, err := requireTokenAndNoteID(c)
		if err != nil {
			return handleError(c, err)
		}

		return c.SendStatus(fiber.StatusNoContent)
	})

	code, _ := doReq(t, app, "/notes")
	if code != fiber.StatusBadRequest {
		t.Fatalf("no id %d", code)
	}

	app2 := fiber.New()
	app2.Get("/notes/:note_id", func(c fiber.Ctx) error {
		_, _, err := requireTokenAndNoteID(c)
		if err != nil {
			return handleError(c, err)
		}

		return c.SendStatus(fiber.StatusNoContent)
	})

	code, _ = doReq(t, app2, "/notes/n1")
	if code != fiber.StatusUnauthorized {
		t.Fatalf("no token %d", code)
	}

	direct := fiber.New()

	var got error

	direct.Get("/check/:note_id", func(c fiber.Ctx) error {
		_, _, got = requireTokenAndNoteID(c)

		return nil
	})
	direct.Get("/check", func(c fiber.Ctx) error {
		c.SetContext(authctx.WithBearerToken(c.Context(), "tok"))
		_, _, got = requireTokenAndNoteID(c)

		return nil
	})

	_, _ = doReq(t, direct, "/check/n1")

	var ferr *fiber.Error
	if !errors.As(got, &ferr) || ferr.Code != fiber.StatusUnauthorized {
		t.Fatalf("direct no token: %v", got)
	}

	_, _ = doReq(t, direct, "/check")
	if !errors.As(got, &ferr) || ferr.Code != fiber.StatusBadRequest {
		t.Fatalf("direct no id: %v", got)
	}
}

func TestNoteToAPIHandleErrorNilAndJSONWriteFail(t *testing.T) {
	t.Parallel()

	if noteToAPI(nil) != nil {
		t.Fatal("expected nil")
	}

	app := fiber.New()
	app.Get("/nil", func(c fiber.Ctx) error {
		return handleError(c, nil)
	})
	app.Get("/badjson", func(c fiber.Ctx) error {
		return jsonResponse(c, fiber.StatusOK, make(chan int))
	})

	code, body := doReq(t, app, "/nil")
	if code != fiber.StatusInternalServerError || body["error"] != msgInternalServerError {
		t.Fatalf("%d %v", code, body)
	}

	code, _ = doReq(t, app, "/badjson")
	if code == fiber.StatusOK {
		t.Fatal("expected json encode failure")
	}
}

func doReq(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		path,
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	raw, _ := io.ReadAll(resp.Body)

	out := map[string]any{}
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &out)
	}

	return resp.StatusCode, out
}
