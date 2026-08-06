//go:build integration

package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/flexer2006/notes-microservices/internal/testkit"
	"github.com/flexer2006/notes-microservices/internal/testkit/integration"
)

func TestMain(m *testing.M) {
	testkit.UseNopLogger()
	m.Run()
}

func TestRedisCacheRoundTrip(t *testing.T) {
	cache := integration.StartRedisCache(t)
	ctx := context.Background()

	testkit.MyErrIs(t, cache.Ping(ctx), nil)

	miss, err := cache.Get(ctx, "missing-key")
	testkit.MyErrIs(t, err, nil)

	if miss != "" {
		t.Fatalf("miss=%q", miss)
	}

	testkit.MyErrIs(t, cache.Set(ctx, "k1", "v1", 0), nil)

	got, err := cache.Get(ctx, "k1")
	testkit.MyErrIs(t, err, nil)

	if got != "v1" {
		t.Fatalf("got=%q", got)
	}

	testkit.MyErrIs(t, cache.Set(ctx, "k2", "v2", 2*time.Second), nil)
	testkit.MyErrIs(t, cache.Delete(ctx, "k1"), nil)

	gone, err := cache.Get(ctx, "k1")
	testkit.MyErrIs(t, err, nil)

	if gone != "" {
		t.Fatalf("deleted=%q", gone)
	}
}
