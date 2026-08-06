package bcrypt_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	bcryptadapter "github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
)

func TestHashAndVerify(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	hash, err := svc.Hash(context.Background(), "password1")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := svc.Verify(context.Background(), "password1", hash)
	if err != nil || !ok {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}

	ok, err = svc.Verify(context.Background(), "wrongpass1", hash)
	if err != nil || ok {
		t.Fatalf("mismatch ok=%v err=%v", ok, err)
	}
}

func TestNewBcryptClampsCost(t *testing.T) {
	t.Parallel()

	low := bcryptadapter.NewBcrypt(bcrypt.MinCost - 1)

	hash, err := low.Hash(context.Background(), "password1")
	if err != nil {
		t.Fatal(err)
	}

	cost, costErr := bcrypt.Cost([]byte(hash))
	if costErr != nil {
		t.Fatal(costErr)
	}

	if cost != bcrypt.DefaultCost {
		t.Fatalf("cost = %d, want default", cost)
	}

	high := bcryptadapter.NewBcrypt(bcrypt.MaxCost + 1)
	if high == nil {
		t.Fatal("nil service")
	}
}

func TestHashEmpty(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	_, err := svc.Hash(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHashTooLong(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	_, err := svc.Hash(context.Background(), strings.Repeat("a", 80))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyEmpty(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	_, err := svc.Verify(context.Background(), "", "x")
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = svc.Verify(context.Background(), "x", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHashCanceledContext(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Hash(ctx, "password1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyCanceledContext(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	hash, err := svc.Hash(context.Background(), "password1")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.Verify(ctx, "password1", hash)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyMalformedHash(t *testing.T) {
	t.Parallel()

	svc := bcryptadapter.NewBcrypt(bcrypt.MinCost)

	_, err := svc.Verify(context.Background(), "password1", "not-a-hash")
	if err == nil {
		t.Fatal("expected error")
	}
}
