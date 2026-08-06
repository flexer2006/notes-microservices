package bcrypt

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var errEmptyPassword = errors.New("password is empty")

type Service struct {
	cost int
}

func NewBcrypt(cost int) *Service {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}

	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}

	return new(Service{cost: cost})
}

func (s *Service) Hash(ctx context.Context, password string) (string, error) {
	if password == "" {
		return "", errEmptyPassword
	}

	ctxErr := ctx.Err()
	if ctxErr != nil {
		return "", ctxErr
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: generate hash: %w", err)
	}

	return string(hashed), nil
}

func (s *Service) Verify(ctx context.Context, password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, errEmptyPassword
	}

	ctxErr := ctx.Err()
	if ctxErr != nil {
		return false, ctxErr
	}

	compareErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if compareErr != nil {
		if errors.Is(compareErr, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}

		return false, fmt.Errorf("bcrypt: compare hash: %w", compareErr)
	}

	return true, nil
}
