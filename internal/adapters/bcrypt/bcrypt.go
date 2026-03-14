package bcrypt

import (
	"context"
	"errors"
	"fmt"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	cost int
}

func NewBcrypt(cost int) ports.PasswordService {
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
		return "", domain.ErrInvalidPassword
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrFailedToGenerateHash, err)
	}
	return string(hashed), nil
}

func (s *Service) Verify(ctx context.Context, password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, domain.ErrInvalidPassword
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", domain.ErrErrorComparingHash, err)
	}
	return true, nil
}
