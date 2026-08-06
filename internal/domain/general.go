package domain

import (
	"regexp"
	"strings"
	"time"
)

const (
	minPasswordLength = 8
	maxUsernameLength = 50
	maxNoteTitleLen   = 255
	maxNoteContentLen = 64 << 10
)

var (
	emailRegex          = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	passwordLetterRegex = regexp.MustCompile(`[a-zA-Z]`)
	passwordDigitRegex  = regexp.MustCompile(`\d`)
)

type Note struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
}

type User struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
}

type TokenPair struct {
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type RefreshToken struct {
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"-"`
	IsRevoked bool      `json:"is_revoked"`
}

func ValidateEmail(email string) error {
	if !emailRegex.MatchString(email) {
		return ErrInvalidEmail
	}

	return nil
}

func ValidatePassword(password string) error {
	if len(password) < minPasswordLength {
		return ErrPasswordTooShort
	}

	if !passwordLetterRegex.MatchString(password) || !passwordDigitRegex.MatchString(password) {
		return ErrPasswordTooWeak
	}

	return nil
}

func NewUser(email, username, passwordHash string) (*User, error) {
	emailErr := ValidateEmail(email)
	if emailErr != nil {
		return nil, emailErr
	}

	if strings.TrimSpace(username) == "" {
		return nil, ErrEmptyUsername
	}

	if len(username) > maxUsernameLength {
		return nil, ErrUsernameTooLong
	}

	return new(User{
		CreatedAt:    time.Time{},
		UpdatedAt:    time.Time{},
		ID:           "",
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
	}), nil
}

func NewNote(userID, title, content string) (*Note, error) {
	if userID == "" {
		return nil, ErrEmptyUserID
	}

	if strings.TrimSpace(title) == "" {
		return nil, ErrEmptyNoteTitle
	}

	if len(title) > maxNoteTitleLen {
		return nil, ErrNoteTitleTooLong
	}

	if len(content) > maxNoteContentLen {
		return nil, ErrNoteContentTooLarge
	}

	now := time.Now().UTC()

	return new(Note{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "",
		UserID:    userID,
		Title:     title,
		Content:   content,
	}), nil
}

func (n *Note) ApplyUpdate(title, content *string) error {
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return ErrEmptyNoteTitle
		}

		if len(*title) > maxNoteTitleLen {
			return ErrNoteTitleTooLong
		}

		n.Title = *title
	}

	if content != nil {
		if len(*content) > maxNoteContentLen {
			return ErrNoteContentTooLarge
		}

		n.Content = *content
	}

	n.UpdatedAt = time.Now().UTC()

	return nil
}
