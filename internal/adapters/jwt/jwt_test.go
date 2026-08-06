package jwt_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

const (
	accessSecret  = "access-secret-key-at-least-32-bytes!!"
	refreshSecret = "refresh-secret-key-at-least-32-bytes!"
)

func TestAccessRoundTrip(t *testing.T) {
	t.Parallel()

	svc := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
	token, exp, err := svc.GenerateAccessToken(context.Background(), "u1", "alice")
	testkit.MyErrIs(t, err, nil)

	if token == "" || exp.IsZero() {
		t.Fatalf("token/exp empty")
	}

	uid, err := svc.ValidateAccessToken(context.Background(), token)
	testkit.MyErrIs(t, err, nil)

	if uid != "u1" {
		t.Fatalf("uid = %q", uid)
	}
}

func TestRefreshRejectedAsAccess(t *testing.T) {
	t.Parallel()

	svc := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
	refresh, _, err := svc.GenerateRefreshToken(context.Background(), "u1")
	testkit.MyErrIs(t, err, nil)

	_, err = svc.ValidateAccessToken(context.Background(), refresh)
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)
}

func TestVerifierAcceptsAccessOnly(t *testing.T) {
	t.Parallel()

	issuer := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
	access, _, err := issuer.GenerateAccessToken(context.Background(), "u1", "alice")
	testkit.MyErrIs(t, err, nil)

	refresh, _, err := issuer.GenerateRefreshToken(context.Background(), "u1")
	testkit.MyErrIs(t, err, nil)

	verifier := jwt.NewAccessVerifier(accessSecret)
	uid, err := verifier.ValidateAccessToken(context.Background(), access)
	testkit.MyErrIs(t, err, nil)

	if uid != "u1" {
		t.Fatalf("uid = %q", uid)
	}

	_, err = verifier.ValidateAccessToken(context.Background(), refresh)
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)

	_, _, err = verifier.GenerateRefreshToken(context.Background(), "u1")
	testkit.MyErrIs(t, err, domain.ErrTokenGeneration)
}

func TestWrongAccessSecret(t *testing.T) {
	t.Parallel()

	issuer := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
	token, _, err := issuer.GenerateAccessToken(context.Background(), "u1", "alice")
	testkit.MyErrIs(t, err, nil)

	other := jwt.NewAccessVerifier("other-secret-key-at-least-32-bytes!!")
	_, err = other.ValidateAccessToken(context.Background(), token)
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)
}

func TestExpiredAccessToken(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc := jwt.NewIssuer(accessSecret, refreshSecret, time.Second, time.Hour)
		token, _, err := svc.GenerateAccessToken(context.Background(), "u1", "alice")
		testkit.MyErrIs(t, err, nil)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		_, err = svc.ValidateAccessToken(context.Background(), token)
		testkit.MyErrIs(t, err, domain.ErrExpiredJWTToken)
	})
}

func TestRefreshTokensUniqueAtSameInstant(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		svc := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
		a, _, err := svc.GenerateRefreshToken(context.Background(), "u1")
		testkit.MyErrIs(t, err, nil)

		b, _, err := svc.GenerateRefreshToken(context.Background(), "u1")
		testkit.MyErrIs(t, err, nil)

		if a == "" || a == b {
			t.Fatalf("refresh tokens not unique: %q %q", a, b)
		}
	})
}

func TestEmptyAccessSecretOnValidate(t *testing.T) {
	t.Parallel()

	issuer := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)
	token, _, err := issuer.GenerateAccessToken(context.Background(), "u1", "alice")
	testkit.MyErrIs(t, err, nil)

	empty := jwt.NewAccessVerifier("")
	_, err = empty.ValidateAccessToken(context.Background(), token)
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)
}

func TestInvalidTokenString(t *testing.T) {
	t.Parallel()

	svc := jwt.NewAccessVerifier(accessSecret)
	_, err := svc.ValidateAccessToken(context.Background(), "not-a-jwt")
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)
}

func TestAccessTokenEmptyUserID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	raw := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id":   "",
		"token_use": "access",
		"sub":       "u1",
		"iat":       now.Unix(),
		"exp":       now.Add(time.Minute).Unix(),
	})

	token, err := raw.SignedString([]byte(accessSecret))
	if err != nil {
		t.Fatal(err)
	}

	svc := jwt.NewAccessVerifier(accessSecret)
	_, err = svc.ValidateAccessToken(context.Background(), token)
	testkit.MyErrIs(t, err, domain.ErrInvalidJWTToken)
}

func FuzzValidateAccessToken(f *testing.F) {
	issuer := jwt.NewIssuer(accessSecret, refreshSecret, time.Minute, time.Hour)

	good, _, err := issuer.GenerateAccessToken(context.Background(), "u1", "alice")
	if err != nil {
		f.Fatal(err)
	}

	f.Add(good)
	f.Add("")
	f.Add("x.y.z")
	f.Fuzz(func(t *testing.T, raw string) {
		svc := jwt.NewAccessVerifier(accessSecret)

		_, err := svc.ValidateAccessToken(context.Background(), raw)
		if err == nil {
			return
		}

		if errors.Is(err, domain.ErrInvalidJWTToken) || errors.Is(err, domain.ErrExpiredJWTToken) {
			return
		}

		t.Fatalf("unexpected err %v", err)
	})
}
