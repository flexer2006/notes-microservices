package authctx

import "context"

type ctxKey string

const (
	bearerTokenKey ctxKey = "bearer_token"
	userIDKey      ctxKey = "user_id"
)

func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenKey, token)
}

func BearerTokenFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if token, ok := ctx.Value(bearerTokenKey).(string); ok {
		return token
	}

	return ""
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if userID, ok := ctx.Value(userIDKey).(string); ok {
		return userID
	}

	return ""
}
