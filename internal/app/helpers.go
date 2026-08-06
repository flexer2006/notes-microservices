package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/flexer2006/notes-microservices/internal/logger"
)

func appLog(ctx context.Context, method string) *logger.Logger {
	return logger.Method(ctx, method)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
