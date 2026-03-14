package main

import (
	"context"
	"fmt"
	"os"

	"github.com/flexer2006/notes-microservices/internal/app"
)

func main() {
	if err := app.StartGateway(context.Background(), ""); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gateway service failed: %v\n", err)
		os.Exit(1)
	}
}
