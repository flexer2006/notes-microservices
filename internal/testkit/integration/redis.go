//go:build integration

package integration

import (
	"context"
	"net/url"
	"strconv"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	redisadapter "github.com/flexer2006/notes-microservices/internal/adapters/redis"
	"github.com/flexer2006/notes-microservices/internal/config"
)

func StartRedisCache(t *testing.T) *redisadapter.Cache {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), containerStartTimeout)
	t.Cleanup(cancel)

	ctr, err := tcredis.Run(
		ctx,
		RedisImage,
		testcontainers.WithCmdArgs("--requirepass", TestRedisPassword),
	)
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}

	testcontainers.CleanupContainer(t, ctr)

	uri, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis uri: %v", err)
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse redis uri: %v", err)
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("redis port: %v", err)
	}

	cfg := new(config.Config)
	cfg.Redis = new(config.RedisConfig{
		Host:            parsed.Hostname(),
		Port:            port,
		Password:        TestRedisPassword,
		DB:              0,
		ConnectTimeout:  redisDialTimeout,
		ReadTimeout:     redisIOTimeout,
		WriteTimeout:    redisIOTimeout,
		IdleTimeout:     redisDefaultTTL,
		MaxConnLifetime: 0,
		PoolSize:        redisPoolSize,
		MinIdle:         redisMinIdle,
		DefaultTTL:      redisDefaultTTL,
	})

	cache, err := redisadapter.NewRedisCache(ctx, cfg)
	if err != nil {
		t.Fatalf("new redis cache: %v", err)
	}

	t.Cleanup(func() {
		closeErr := cache.Close()
		if closeErr != nil {
			t.Errorf("close redis: %v", closeErr)
		}
	})

	return cache
}
