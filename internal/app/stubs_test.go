package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

var (
	errBoom = errors.New("boom")
	errMiss = errors.New("miss")
)

type stubUserRepo struct {
	create      func(context.Context, *domain.User) (*domain.User, error)
	findByID    func(context.Context, string) (*domain.User, error)
	findByEmail func(context.Context, string) (*domain.User, error)
}

func (s *stubUserRepo) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	return s.create(ctx, u)
}

func (s *stubUserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	return s.findByID(ctx, id)
}

func (s *stubUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.findByEmail(ctx, email)
}

func (*stubUserRepo) Update(context.Context, *domain.User) (*domain.User, error) {
	panic("unexpected")
}

func (*stubUserRepo) Delete(context.Context, string) error { panic("unexpected") }

type stubTokenRepo struct {
	store     func(context.Context, *domain.RefreshToken) error
	find      func(context.Context, string) (*domain.RefreshToken, error)
	revoke    func(context.Context, string) error
	revokeAll func(context.Context, string) error
	rotate    func(context.Context, string, *domain.RefreshToken) (*domain.RefreshToken, error)
}

func (s *stubTokenRepo) StoreRefreshToken(ctx context.Context, tok *domain.RefreshToken) error {
	return s.store(ctx, tok)
}

func (s *stubTokenRepo) FindByToken(
	ctx context.Context,
	hash string,
) (*domain.RefreshToken, error) {
	return s.find(ctx, hash)
}

func (s *stubTokenRepo) RevokeToken(ctx context.Context, hash string) error {
	return s.revoke(ctx, hash)
}

func (s *stubTokenRepo) RotateRefreshToken(
	ctx context.Context,
	old string,
	neu *domain.RefreshToken,
) (*domain.RefreshToken, error) {
	return s.rotate(ctx, old, neu)
}

func (*stubTokenRepo) ConsumeActiveToken(context.Context, string) (*domain.RefreshToken, error) {
	panic("unexpected")
}

func (s *stubTokenRepo) RevokeAllUserTokens(ctx context.Context, userID string) error {
	if s.revokeAll != nil {
		return s.revokeAll(ctx, userID)
	}

	return nil
}

func (*stubTokenRepo) CleanupExpiredTokens(context.Context) error { panic("unexpected") }

func (*stubTokenRepo) FindUserTokens(context.Context, string) ([]*domain.RefreshToken, error) {
	panic("unexpected")
}

type stubPassword struct {
	hash   func(context.Context, string) (string, error)
	verify func(context.Context, string, string) (bool, error)
}

func (s *stubPassword) Hash(ctx context.Context, p string) (string, error) {
	return s.hash(ctx, p)
}

func (s *stubPassword) Verify(ctx context.Context, p, h string) (bool, error) {
	return s.verify(ctx, p, h)
}

type stubTokens struct {
	access  func(context.Context, string, string) (string, time.Time, error)
	refresh func(context.Context, string) (string, time.Time, error)
}

func (s *stubTokens) GenerateAccessToken(
	ctx context.Context,
	uid, name string,
) (string, time.Time, error) {
	return s.access(ctx, uid, name)
}

func (s *stubTokens) GenerateRefreshToken(
	ctx context.Context,
	uid string,
) (string, time.Time, error) {
	return s.refresh(ctx, uid)
}

func (*stubTokens) ValidateAccessToken(context.Context, string) (string, error) {
	panic("unexpected")
}

func okTokens() *stubTokens {
	exp := time.Now().UTC().Add(time.Hour)

	return new(stubTokens{
		access: func(context.Context, string, string) (string, time.Time, error) {
			return "access", exp, nil
		},
		refresh: func(context.Context, string) (string, time.Time, error) {
			return "refresh", exp, nil
		},
	})
}

func okPassword() *stubPassword {
	return new(stubPassword{
		hash: func(_ context.Context, p string) (string, error) { return "h:" + p, nil },
		verify: func(_ context.Context, p, h string) (bool, error) {
			return h == "h:"+p, nil
		},
	})
}

type stubNotes struct {
	create func(context.Context, *domain.Note) (*domain.Note, error)
	get    func(context.Context, string, string) (*domain.Note, error)
	list   func(context.Context, string, int, int) ([]*domain.Note, int, error)
	update func(context.Context, *domain.Note) error
	delete func(context.Context, string, string) error
}

func (s *stubNotes) Create(ctx context.Context, n *domain.Note) (*domain.Note, error) {
	return s.create(ctx, n)
}

func (s *stubNotes) GetByID(ctx context.Context, id, uid string) (*domain.Note, error) {
	return s.get(ctx, id, uid)
}

func (s *stubNotes) ListByUserID(
	ctx context.Context,
	uid string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	return s.list(ctx, uid, limit, offset)
}

func (s *stubNotes) Update(ctx context.Context, n *domain.Note) error {
	return s.update(ctx, n)
}

func (s *stubNotes) Delete(ctx context.Context, id, uid string) error {
	return s.delete(ctx, id, uid)
}

type stubCache struct {
	get func(context.Context, string) (string, error)
	set func(context.Context, string, string, time.Duration) error
}

func (s *stubCache) Get(ctx context.Context, key string) (string, error) {
	if s.get != nil {
		return s.get(ctx, key)
	}

	return "", errMiss
}

func (s *stubCache) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	if s.set != nil {
		return s.set(ctx, key, val, ttl)
	}

	return nil
}

func (*stubCache) Delete(context.Context, string) error { return nil }

func (*stubCache) Ping(context.Context) error { return nil }

func (*stubCache) Close() error { return nil }

type stubAuthBackend struct {
	register func(context.Context, string, string, string) (*domain.TokenPair, error)
	login    func(context.Context, string, string) (*domain.TokenPair, error)
	refresh  func(context.Context, string) (*domain.TokenPair, error)
	logout   func(context.Context, string) error
	profile  func(context.Context, string) (*domain.User, error)
}

func (s *stubAuthBackend) Register(
	ctx context.Context,
	email, user, pass string,
) (*domain.TokenPair, error) {
	return s.register(ctx, email, user, pass)
}

func (s *stubAuthBackend) Login(
	ctx context.Context,
	email, pass string,
) (*domain.TokenPair, error) {
	return s.login(ctx, email, pass)
}

func (s *stubAuthBackend) RefreshTokens(
	ctx context.Context,
	tok string,
) (*domain.TokenPair, error) {
	return s.refresh(ctx, tok)
}

func (s *stubAuthBackend) Logout(ctx context.Context, tok string) error {
	return s.logout(ctx, tok)
}

func (s *stubAuthBackend) GetUserProfile(ctx context.Context, tok string) (*domain.User, error) {
	return s.profile(ctx, tok)
}

func (*stubAuthBackend) Close() error { return nil }

type stubNotesBackend struct {
	create func(context.Context, string, string, string) (*domain.Note, error)
	get    func(context.Context, string, string) (*domain.Note, error)
	list   func(context.Context, string, int, int) ([]*domain.Note, int, error)
	update func(context.Context, string, string, *string, *string) (*domain.Note, error)
	delete func(context.Context, string, string) error
}

func (s *stubNotesBackend) CreateNote(
	ctx context.Context,
	tok, title, content string,
) (*domain.Note, error) {
	return s.create(ctx, tok, title, content)
}

func (s *stubNotesBackend) GetNote(ctx context.Context, tok, id string) (*domain.Note, error) {
	return s.get(ctx, tok, id)
}

func (s *stubNotesBackend) ListNotes(
	ctx context.Context,
	tok string,
	limit, offset int,
) ([]*domain.Note, int, error) {
	return s.list(ctx, tok, limit, offset)
}

func (s *stubNotesBackend) UpdateNote(
	ctx context.Context,
	tok, id string,
	title, content *string,
) (*domain.Note, error) {
	return s.update(ctx, tok, id, title, content)
}

func (s *stubNotesBackend) DeleteNote(ctx context.Context, tok, id string) error {
	return s.delete(ctx, tok, id)
}

func (*stubNotesBackend) Close() error { return nil }
