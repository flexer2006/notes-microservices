package config_test

import (
	"testing"

	"github.com/flexer2006/notes-microservices/internal/config"
)

func TestResolvedSecrets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		cfg         *config.JWTConfig
		wantAccess  string
		wantRefresh string
	}{
		{name: "nil", cfg: nil, wantAccess: "", wantRefresh: ""},
		{
			name: "split",
			cfg: new(config.JWTConfig{
				AccessSecretKey:  " access ",
				RefreshSecretKey: " refresh ",
				SecretKey:        "legacy",
			}),
			wantAccess:  "access",
			wantRefresh: "refresh",
		},
		{
			name: "legacy_fallback",
			cfg: new(config.JWTConfig{
				SecretKey: " legacy ",
			}),
			wantAccess:  "legacy",
			wantRefresh: "legacy",
		},
		{
			name:        "empty",
			cfg:         new(config.JWTConfig{}),
			wantAccess:  "",
			wantRefresh: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.cfg.ResolvedAccessSecret(); got != tc.wantAccess {
				t.Fatalf("access = %q, want %q", got, tc.wantAccess)
			}

			if got := tc.cfg.ResolvedRefreshSecret(); got != tc.wantRefresh {
				t.Fatalf("refresh = %q, want %q", got, tc.wantRefresh)
			}
		})
	}
}
