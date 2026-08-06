//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pgadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
	"github.com/flexer2006/notes-microservices/internal/testkit/integration"
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

func TestAuthRepositories(t *testing.T) {
	db := integration.StartPostgres(t, "auth")
	factory := pgadapter.NewAuthRepositoryFactory(db.Pool())
	users := factory.UserRepository()
	tokens := factory.TokenRepository()
	ctx := context.Background()

	created, err := users.Create(ctx, new(domain.User{
		Email: "alice@example.com", Username: "alice", PasswordHash: "hash1",
	}))
	testkit.MyErrIs(t, err, nil)

	if created.ID == "" {
		t.Fatal("empty user id")
	}

	found, err := users.FindByEmail(ctx, "alice@example.com")
	testkit.MyErrIs(t, err, nil)

	if found.Username != "alice" {
		t.Fatalf("%+v", found)
	}

	_, err = users.Create(ctx, new(domain.User{
		Email: "alice@example.com", Username: "alice2", PasswordHash: "hash2",
	}))
	emailDup := errors.Is(err, domain.ErrEmailAlreadyExists)

	userDup := errors.Is(err, domain.ErrUserAlreadyExists)
	if !emailDup && !userDup {
		t.Fatalf("unique email: %v", err)
	}

	expires := time.Now().UTC().Add(time.Hour)
	storeErr := tokens.StoreRefreshToken(ctx, new(domain.RefreshToken{
		UserID: created.ID, Token: "tok-a", ExpiresAt: expires,
	}))
	testkit.MyErrIs(t, storeErr, nil)

	got, err := tokens.FindByToken(ctx, "tok-a")
	testkit.MyErrIs(t, err, nil)

	if got.UserID != created.ID || got.IsRevoked {
		t.Fatalf("%+v", got)
	}

	consumed, err := tokens.ConsumeActiveToken(ctx, "tok-a")
	testkit.MyErrIs(t, err, nil)

	if consumed.Token != "tok-a" {
		t.Fatalf("%+v", consumed)
	}

	_, err = tokens.ConsumeActiveToken(ctx, "tok-a")
	if err == nil {
		t.Fatal("expected consume of revoked token to fail")
	}
}

func TestNoteRepository(t *testing.T) {
	db := integration.StartPostgres(t, "notes")
	repo := pgadapter.NewNoteRepository(db.Pool())
	ctx := context.Background()
	userID := "11111111-1111-1111-1111-111111111111"

	note, err := repo.Create(ctx, new(domain.Note{
		UserID: userID, Title: "t1", Content: "c1",
	}))
	testkit.MyErrIs(t, err, nil)

	got, err := repo.GetByID(ctx, note.ID, userID)
	testkit.MyErrIs(t, err, nil)

	if got.Title != "t1" {
		t.Fatalf("%+v", got)
	}

	_, err = repo.GetByID(ctx, note.ID, "22222222-2222-2222-2222-222222222222")
	if !errors.Is(err, domain.ErrNoteNotFound) {
		t.Fatalf("ownership: %v", err)
	}

	note.Title = "t2"
	note.Content = "c2"
	testkit.MyErrIs(t, repo.Update(ctx, note), nil)

	list, total, err := repo.ListByUserID(ctx, userID, 10, 0)
	testkit.MyErrIs(t, err, nil)

	if total != 1 || len(list) != 1 || list[0].Title != "t2" {
		t.Fatalf("list total=%d n=%d", total, len(list))
	}

	testkit.MyErrIs(t, repo.Delete(ctx, note.ID, userID), nil)

	_, err = repo.GetByID(ctx, note.ID, userID)
	if !errors.Is(err, domain.ErrNoteNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}
