package di

import "github.com/flexer2006/notes-microservices/internal/app"

// Backward-compatibility wrapper for cmd packages that still import internal/di.
// New code should import internal/app directly.

var (
	Load = app.Load
	Wait = app.Wait
)
