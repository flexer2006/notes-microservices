package main

import (
	"context"
	"fmt"
	"os"

	"github.com/flexer2006/notes-microservices/internal/app"
)

func main() {
	if err := app.StartAuth(context.Background(), ""); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "auth service failed: %v\n", err)
		os.Exit(1)
	}
}
