package grpc

import (
	"testing"

	"github.com/flexer2006/notes-microservices/internal/config"
)

func TestReflectionEnabled(t *testing.T) {
	t.Parallel()

	if new(Server{}).reflectionEnabled() {
		t.Fatal("nil cfg")
	}

	if new(Server{cfg: new(config.Config{})}).reflectionEnabled() {
		t.Fatal("nil grpc cfg")
	}

	on := new(Server{cfg: new(config.Config{GRPC: new(config.GRPCConfig{Reflection: true})})})
	if !on.reflectionEnabled() {
		t.Fatal("want true")
	}
}
