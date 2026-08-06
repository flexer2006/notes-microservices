package main

import (
	"context"
	"fmt"
	"os"

	"github.com/flexer2006/notes-microservices/internal/bootstrap"
)

func main() {
	err := bootstrap.StartGateway(context.Background(), "")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gateway failed: %v\n", err)

		os.Exit(1)
	}
}
