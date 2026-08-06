package app_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/flexer2006/notes-microservices/internal/app"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

const (
	testEmail = "a@ex.com"
	testUser  = "alice"
	testPass  = "password1"
)

func sampleUser() *domain.User {
	return new(domain.User{
		ID: "u1", Email: testEmail, Username: testUser, PasswordHash: "h:" + testPass,
	})
}

func TestAuthRegister(t *testing.T) {
	t.Parallel()

	users := new(stubUserRepo{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		create: func(_ context.Context, u *domain.User) (*domain.User, error) {
			u.ID = "u1"

			return u, nil
		},
	})
	tokens := new(stubTokenRepo{
		store: func(context.Context, *domain.RefreshToken) error { return nil },
	})
	uc := app.NewAuthUseCase(users, tokens, okPassword(), okTokens())

	pair, err := uc.Register(context.Background(), testEmail, testUser, testPass)
	testkit.MyErrIs(t, err, nil)

	if pair.UserID != "u1" || pair.AccessToken == "" {
		t.Fatalf("%+v", pair)
	}

	for _, tc := range []struct {
		email, user, pass string
		want              error
	}{
		{"bad", testUser, testPass, domain.ErrInvalidEmail},
		{testEmail, testUser, "short", domain.ErrPasswordTooShort},
		{testEmail, "  ", testPass, domain.ErrEmptyUsername},
	} {
		_, err := uc.Register(context.Background(), tc.email, tc.user, tc.pass)
		testkit.MyErrIs(t, err, tc.want)
	}
}

func TestAuthRegisterFailures(t *testing.T) {
	t.Parallel()

	missing := func(context.Context, string) (*domain.User, error) {
		return nil, domain.ErrUserNotFound
	}
	created := func(_ context.Context, u *domain.User) (*domain.User, error) {
		u.ID = "u1"

		return u, nil
	}

	t.Run("exists", func(t *testing.T) {
		t.Parallel()

		uc := app.NewAuthUseCase(new(stubUserRepo{
			findByEmail: func(context.Context, string) (*domain.User, error) {
				return sampleUser(), nil
			},
		}), new(stubTokenRepo{}), okPassword(), okTokens())
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, domain.ErrEmailAlreadyExists)
	})

	t.Run("find", func(t *testing.T) {
		t.Parallel()

		uc := app.NewAuthUseCase(new(stubUserRepo{
			findByEmail: func(context.Context, string) (*domain.User, error) {
				return nil, errBoom
			},
		}), new(stubTokenRepo{}), okPassword(), okTokens())
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("hash", func(t *testing.T) {
		t.Parallel()

		pw := okPassword()
		pw.hash = func(context.Context, string) (string, error) { return "", errBoom }
		uc := app.NewAuthUseCase(
			new(stubUserRepo{findByEmail: missing}),
			new(stubTokenRepo{}),
			pw,
			okTokens(),
		)
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		uc := app.NewAuthUseCase(new(stubUserRepo{
			findByEmail: missing,
			create: func(context.Context, *domain.User) (*domain.User, error) {
				return nil, errBoom
			},
		}), new(stubTokenRepo{}), okPassword(), okTokens())
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("tokens", func(t *testing.T) {
		t.Parallel()

		tok := okTokens()
		tok.access = func(context.Context, string, string) (string, time.Time, error) {
			return "", time.Time{}, errBoom
		}
		uc := app.NewAuthUseCase(
			new(stubUserRepo{findByEmail: missing, create: created}),
			new(stubTokenRepo{
				store: func(context.Context, *domain.RefreshToken) error { return nil },
			}),
			okPassword(),
			tok,
		)
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, errBoom)
	})

	t.Run("store", func(t *testing.T) {
		t.Parallel()

		uc := app.NewAuthUseCase(
			new(stubUserRepo{findByEmail: missing, create: created}),
			new(stubTokenRepo{
				store: func(context.Context, *domain.RefreshToken) error { return errBoom },
			}),
			okPassword(),
			okTokens(),
		)
		_, err := uc.Register(context.Background(), testEmail, testUser, testPass)
		testkit.MyErrIs(t, err, errBoom)
	})
}

func TestAuthLogin(t *testing.T) {
	t.Parallel()

	users := new(stubUserRepo{
		findByEmail: func(context.Context, string) (*domain.User, error) {
			return sampleUser(), nil
		},
	})
	tokens := new(stubTokenRepo{
		store: func(context.Context, *domain.RefreshToken) error { return nil },
	})
	uc := app.NewAuthUseCase(users, tokens, okPassword(), okTokens())

	pair, err := uc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, nil)

	if pair.UserID != "u1" {
		t.Fatalf("%+v", pair)
	}

	users.findByEmail = func(context.Context, string) (*domain.User, error) {
		return nil, domain.ErrUserNotFound
	}
	_, err = uc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, domain.ErrInvalidCredentials)

	users.findByEmail = func(context.Context, string) (*domain.User, error) { return nil, errBoom }
	_, err = uc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, errBoom)

	users.findByEmail = func(context.Context, string) (*domain.User, error) {
		return sampleUser(), nil
	}
	pw := okPassword()
	pw.verify = func(context.Context, string, string) (bool, error) { return false, errBoom }
	uc = app.NewAuthUseCase(users, tokens, pw, okTokens())
	_, err = uc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, errBoom)

	uc = app.NewAuthUseCase(users, tokens, okPassword(), okTokens())
	_, err = uc.Login(context.Background(), testEmail, "wrongpass1")
	testkit.MyErrIs(t, err, domain.ErrInvalidCredentials)

	tok := okTokens()
	tok.refresh = func(context.Context, string) (string, time.Time, error) {
		return "", time.Time{}, errBoom
	}
	uc = app.NewAuthUseCase(users, tokens, okPassword(), tok)
	_, err = uc.Login(context.Background(), testEmail, testPass)
	testkit.MyErrIs(t, err, errBoom)
}

func TestAuthRefresh(t *testing.T) {
	t.Parallel()

	raw := "refresh-raw"
	active := new(domain.RefreshToken{
		UserID: "u1", Token: "any", ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	users := new(stubUserRepo{
		findByID: func(context.Context, string) (*domain.User, error) { return sampleUser(), nil },
	})
	repo := new(stubTokenRepo{
		find: func(context.Context, string) (*domain.RefreshToken, error) { return active, nil },
		rotate: func(
			_ context.Context,
			_ string,
			neu *domain.RefreshToken,
		) (*domain.RefreshToken, error) {
			return neu, nil
		},
	})
	uc := app.NewAuthUseCase(users, repo, okPassword(), okTokens())

	pair, err := uc.RefreshTokens(context.Background(), raw)
	testkit.MyErrIs(t, err, nil)

	if pair.RefreshToken == "" {
		t.Fatal("empty refresh")
	}

	type tc struct {
		name string
		repo *stubTokenRepo
		user *stubUserRepo
		tok  *stubTokens
		want error
	}

	cases := []tc{
		{
			name: "missing",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return nil, domain.ErrInvalidRefreshToken
				},
			}),
			want: domain.ErrInvalidRefreshToken,
		},
		{
			name: "find_boom",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return nil, errBoom
				},
			}),
			want: errBoom,
		},
		{
			name: "revoked",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return new(domain.RefreshToken{
						UserID: "u1", IsRevoked: true, ExpiresAt: time.Now().Add(time.Hour),
					}), nil
				},
				revokeAll: func(_ context.Context, userID string) error {
					if userID != "u1" {
						return errBoom
					}

					return nil
				},
			}),
			want: domain.ErrRevokedRefreshToken,
		},
		{
			name: "user",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return active, nil
				},
			}),
			user: new(stubUserRepo{
				findByID: func(context.Context, string) (*domain.User, error) {
					return nil, domain.ErrUserNotFound
				},
			}),
			want: domain.ErrUserNotFound,
		},
		{
			name: "access",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return active, nil
				},
			}),
			tok: new(stubTokens{
				access: func(context.Context, string, string) (string, time.Time, error) {
					return "", time.Time{}, errBoom
				},
				refresh: okTokens().refresh,
			}),
			want: errBoom,
		},
		{
			name: "refresh_gen",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return active, nil
				},
			}),
			tok: new(stubTokens{
				access: okTokens().access,
				refresh: func(context.Context, string) (string, time.Time, error) {
					return "", time.Time{}, errBoom
				},
			}),
			want: errBoom,
		},
		{
			name: "rotate_invalid",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return active, nil
				},
				rotate: func(context.Context, string, *domain.RefreshToken) (*domain.RefreshToken, error) {
					return nil, domain.ErrInvalidRefreshToken
				},
				revokeAll: func(_ context.Context, userID string) error {
					if userID != "u1" {
						return errBoom
					}

					return nil
				},
			}),
			want: domain.ErrRevokedRefreshToken,
		},
		{
			name: "reuse_revoke_all_fails",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return new(domain.RefreshToken{
						UserID: "u1", IsRevoked: true, ExpiresAt: time.Now().Add(time.Hour),
					}), nil
				},
				revokeAll: func(context.Context, string) error { return errBoom },
			}),
			want: errBoom,
		},
		{
			name: "rotate_boom",
			repo: new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return active, nil
				},
				rotate: func(context.Context, string, *domain.RefreshToken) (*domain.RefreshToken, error) {
					return nil, errBoom
				},
			}),
			want: errBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			user, tok := users, okTokens()
			if tc.user != nil {
				user = tc.user
			}

			if tc.tok != nil {
				tok = tc.tok
			}

			uc := app.NewAuthUseCase(user, tc.repo, okPassword(), tok)
			_, err := uc.RefreshTokens(context.Background(), raw)
			testkit.MyErrIs(t, err, tc.want)
		})
	}

	t.Run("expired", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			uc := app.NewAuthUseCase(users, new(stubTokenRepo{
				find: func(context.Context, string) (*domain.RefreshToken, error) {
					return new(domain.RefreshToken{
						UserID: "u1", ExpiresAt: time.Now().Add(-time.Second),
					}), nil
				},
			}), okPassword(), okTokens())
			_, err := uc.RefreshTokens(context.Background(), raw)
			testkit.MyErrIs(t, err, domain.ErrInvalidRefreshToken)
		})
	})
}

func TestAuthLogout(t *testing.T) {
	t.Parallel()

	repo := new(stubTokenRepo{
		find: func(context.Context, string) (*domain.RefreshToken, error) {
			return new(domain.RefreshToken{UserID: "u1"}), nil
		},
		revoke: func(context.Context, string) error { return nil },
	})
	uc := app.NewAuthUseCase(new(stubUserRepo{}), repo, okPassword(), okTokens())

	err := uc.Logout(context.Background(), "r")
	testkit.MyErrIs(t, err, nil)

	repo.revoke = func(context.Context, string) error { return domain.ErrInvalidRefreshToken }
	err = uc.Logout(context.Background(), "r")
	testkit.MyErrIs(t, err, nil)

	repo.find = func(context.Context, string) (*domain.RefreshToken, error) {
		return nil, domain.ErrInvalidRefreshToken
	}
	repo.revoke = func(context.Context, string) error { return errBoom }
	err = uc.Logout(context.Background(), "r")
	testkit.MyErrIs(t, err, errBoom)
}

func TestUserProfile(t *testing.T) {
	t.Parallel()

	uc := app.NewUserUseCase(new(stubUserRepo{
		findByID: func(context.Context, string) (*domain.User, error) { return sampleUser(), nil },
	}))

	u, err := uc.GetUserProfile(context.Background(), "u1")
	testkit.MyErrIs(t, err, nil)

	if u.Username != testUser {
		t.Fatalf("%+v", u)
	}

	_, err = uc.GetUserProfile(context.Background(), "")
	testkit.MyErrIs(t, err, domain.ErrEmptyUserID)

	uc = app.NewUserUseCase(new(stubUserRepo{
		findByID: func(context.Context, string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
	}))
	_, err = uc.GetUserProfile(context.Background(), "missing")
	testkit.MyErrIs(t, err, domain.ErrUserNotFound)
}
