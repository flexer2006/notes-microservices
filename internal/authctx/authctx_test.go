package authctx_test

import (
	"context"
	"testing"

	"github.com/flexer2006/notes-microservices/internal/authctx"
)

func TestBearerTokenRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "set", token: "abc", want: "abc"},
		{name: "empty_token", token: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := authctx.BearerTokenFrom(authctx.WithBearerToken(context.Background(), tc.token))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBearerTokenFromNil(t *testing.T) {
	t.Parallel()

	var ctx context.Context
	if got := authctx.BearerTokenFrom(ctx); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUserIDRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		userID string
		want   string
	}{
		{name: "set", userID: "u1", want: "u1"},
		{name: "empty", userID: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := authctx.UserIDFrom(authctx.WithUserID(context.Background(), tc.userID))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserIDFromNil(t *testing.T) {
	t.Parallel()

	var ctx context.Context
	if got := authctx.UserIDFrom(ctx); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestWrongTypeIgnored(t *testing.T) {
	t.Parallel()

	type otherKey string

	ctx := context.WithValue(context.Background(), otherKey("bearer_token"), 123)
	if got := authctx.BearerTokenFrom(ctx); got != "" {
		t.Fatalf("got %q", got)
	}

	ctx = context.WithValue(context.Background(), otherKey("user_id"), 123)
	if got := authctx.UserIDFrom(ctx); got != "" {
		t.Fatalf("got %q", got)
	}
}
