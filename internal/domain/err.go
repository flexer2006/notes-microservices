package domain

import "errors"

var (
	ErrEmptyUserID      = errors.New("user ID cannot be empty")
	ErrInvalidEmail     = errors.New("invalid email format")
	ErrEmptyUsername    = errors.New("username cannot be empty")
	ErrPasswordTooShort = errors.New("password must contain at least 8 characters")
	ErrPasswordTooWeak  = errors.New("password must contain at least one letter and one digit")
	ErrInvalidParams    = errors.New("invalid parameters")

	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrEmailAlreadyExists  = errors.New("user with this email already exists")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRevokedRefreshToken = errors.New("refresh token has been revoked")
	ErrTokenGeneration     = errors.New("failed to generate authentication tokens")
	ErrInvalidJWTToken     = errors.New("invalid JWT token")
	ErrExpiredJWTToken     = errors.New("JWT token has expired")
	ErrUnauthorized        = errors.New("unauthorized access")

	ErrNoteNotFound           = errors.New("note not found")
	ErrNoteNotFoundOrNotOwned = errors.New("note not found or not owned by user")
	ErrEmptyNoteTitle         = errors.New("note title cannot be empty")
	ErrNoteContentTooLarge    = errors.New("note content exceeds maximum size")
	ErrNoteTitleTooLong       = errors.New("note title exceeds maximum length")
	ErrUsernameTooLong        = errors.New("username exceeds maximum length")

	ErrServiceUnavailable = errors.New("service unavailable")
)
