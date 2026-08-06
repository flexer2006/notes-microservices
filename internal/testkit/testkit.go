package testkit

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/logger"
)

func MyErrIs(t *testing.T, got, want error) {
	t.Helper()

	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func UseNopLogger() {
	logger.SetGlobalLogger(new(logger.Logger{Logger: zap.NewNop()}))
}
