package http

import (
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/gofiber/fiber/v3"
)

func SetupRouter(app *fiber.App, authService ports.AuthService, notesService ports.NotesService) {
	authHandler := NewAuthHandler(authService)
	notesHandler := NewNotesHandler(notesService)
	app.Use(NewLoggerMiddleware())
	app.Use(NewRecoveryMiddleware())
	apiV1 := app.Group("/api/v1")
	authRoutes := apiV1.Group("/auth")
	authRoutes.Post("/register", authHandler.Register)
	authRoutes.Post("/login", authHandler.Login)
	authRoutes.Post("/refresh", authHandler.RefreshTokens)
	authRoutes.Post("/logout", authHandler.Logout)
	userRoutes := apiV1.Group("/user")
	userRoutes.Use(NewAuthMiddleware())
	userRoutes.Get("/profile", authHandler.GetProfile)
	notesRoutes := apiV1.Group("/notes")
	notesRoutes.Use(NewAuthMiddleware())
	notesRoutes.Post("/", notesHandler.CreateNote)
	notesRoutes.Get("/:note_id", notesHandler.GetNote)
	notesRoutes.Get("/", notesHandler.ListNotes)
	notesRoutes.Patch("/:note_id", notesHandler.UpdateNote)
	notesRoutes.Put("/:note_id", notesHandler.UpdateNote)
	notesRoutes.Delete("/:note_id", notesHandler.DeleteNote)
	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Route not found",
		})
	})
}
