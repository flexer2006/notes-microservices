package http

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"github.com/flexer2006/notes-microservices/internal/ports"
)

const (
	loginRateLimitPerMinute    = 10
	registerRateLimitPerMinute = 5
	refreshRateLimitPerMinute  = 20
	logoutRateLimitPerMinute   = 20
)

func SetupRouter(
	app *fiber.App,
	authService ports.AuthService,
	notesService ports.NotesService,
	cache ports.Cache,
) {
	authHandler := NewAuthHandler(authService)
	notesHandler := NewNotesHandler(notesService)

	app.Use(NewRequestIDMiddleware())
	app.Use(NewLoggerMiddleware())
	app.Use(NewRecoveryMiddleware())

	app.Get("/health", func(fctx fiber.Ctx) error {
		return fctx.Status(fiber.StatusOK).JSON(fiber.Map{statusKey: "ok"})
	})
	app.Get("/ready", func(fctx fiber.Ctx) error {
		err := cache.Ping(fctx.Context())
		if err != nil {
			return fctx.Status(fiber.StatusServiceUnavailable).
				JSON(fiber.Map{statusKey: "unavailable"})
		}

		return fctx.Status(fiber.StatusOK).JSON(fiber.Map{statusKey: "ready"})
	})

	apiV1 := app.Group("/api/v1")

	limitReached := func(fctx fiber.Ctx) error {
		return fctx.Status(fiber.StatusTooManyRequests).
			JSON(fiber.Map{errorKey: "too many requests"})
	}

	loginLimiter := limiter.New(limiter.Config{
		Max:          loginRateLimitPerMinute,
		Expiration:   time.Minute,
		LimitReached: limitReached,
	})
	registerLimiter := limiter.New(limiter.Config{
		Max:          registerRateLimitPerMinute,
		Expiration:   time.Minute,
		LimitReached: limitReached,
	})
	refreshLimiter := limiter.New(limiter.Config{
		Max:          refreshRateLimitPerMinute,
		Expiration:   time.Minute,
		LimitReached: limitReached,
	})
	logoutLimiter := limiter.New(limiter.Config{
		Max:          logoutRateLimitPerMinute,
		Expiration:   time.Minute,
		LimitReached: limitReached,
	})

	authRoutes := apiV1.Group("/auth")
	authRoutes.Post("/register", registerLimiter, authHandler.Register)
	authRoutes.Post("/login", loginLimiter, authHandler.Login)
	authRoutes.Post("/refresh", refreshLimiter, authHandler.RefreshTokens)
	authRoutes.Post("/logout", logoutLimiter, authHandler.Logout)

	userRoutes := apiV1.Group("/user")
	userRoutes.Use(NewAuthMiddleware())
	userRoutes.Get("/profile", authHandler.GetProfile)

	notesRoutes := apiV1.Group("/notes")
	notesRoutes.Use(NewAuthMiddleware())
	notesRoutes.Post("/", notesHandler.CreateNote)
	notesRoutes.Get("/:note_id", notesHandler.GetNote)
	notesRoutes.Get("/", notesHandler.ListNotes)
	notesRoutes.Patch("/:note_id", notesHandler.UpdateNote)
	notesRoutes.Put("/:note_id", notesHandler.ReplaceNote)
	notesRoutes.Delete("/:note_id", notesHandler.DeleteNote)

	app.Use(func(fctx fiber.Ctx) error {
		return fctx.Status(fiber.StatusNotFound).JSON(fiber.Map{errorKey: "route not found"})
	})
}
