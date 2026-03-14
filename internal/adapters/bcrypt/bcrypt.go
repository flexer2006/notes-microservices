package bcrypt

import (
	"context"
	"errors"
	"fmt"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"golang.org/x/crypto/bcrypt"
)

const (
	errMsgFailedToGenerateHash = "failed to generate password hash"
	errMsgErrorComparingHash   = "error comparing password with hash"
	errMsgPasswordTooShort     = "password is too short"
)

type ServiceBcrypt struct {
	cost int
}

func NewBcrypt(cost int) ports.PasswordService {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	return &ServiceBcrypt{cost: cost}
}

func (s *ServiceBcrypt) Hash(_ context.Context, password string) (string, error) {
	if password == "" {
		return "", domain.ErrInvalidPassword
	}
	if len(password) < domain.MinPasswordLength {
		return "", fmt.Errorf("%s: %w", errMsgPasswordTooShort, domain.ErrInvalidPassword)
	}
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", fmt.Errorf("%s: %w", errMsgFailedToGenerateHash, domain.ErrHashingFailed)
	}
	return string(hashedBytes), nil
}

func (s *ServiceBcrypt) Verify(_ context.Context, password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, domain.ErrInvalidPassword
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", errMsgErrorComparingHash, err)
	}
	return true, nil
}
