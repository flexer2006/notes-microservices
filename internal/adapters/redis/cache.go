package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	config "github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	LogMethodGet        = "get"
	LogMethodSet        = "set"
	LogMethodDelete     = "delete"
	LogMethodClose      = "close"
	ErrorFailedToGet    = "failed to get value from redis"
	ErrorFailedToSet    = "failed to set value in redis"
	ErrorFailedToDelete = "failed to delete value from redis"
	ErrorFailedToClose  = "failed to close redis connection"
)

type RedisCache struct {
	client     *redis.Client
	defaultTTL time.Duration
}

func NewRedisCache(ctx context.Context, cfg *config.RedisConfig) (ports.Cache, error) {
	client := redis.NewClient(new(redis.Options{
		Addr:            cfg.GetAddressString(),
		Password:        cfg.Password,
		DB:              cfg.DB,
		DialTimeout:     cfg.ConnectTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdle,
		ConnMaxIdleTime: cfg.IdleTimeout,
		ConnMaxLifetime: cfg.MaxConnLifetime,
	}))
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	return new(RedisCache{
		client:     client,
		defaultTTL: cfg.DefaultTTL,
	}), nil
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	log := logger.Log(ctx).With(zap.String("method", LogMethodGet), zap.String("key", key))
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		log.Error(ctx, ErrorFailedToGet, zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrorFailedToGet, err)
	}
	return value, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	log := logger.Log(ctx).With(zap.String("method", LogMethodSet), zap.String("key", key))
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		log.Error(ctx, ErrorFailedToSet, zap.Error(err))
		return fmt.Errorf("%s: %w", ErrorFailedToSet, err)
	}
	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	log := logger.Log(ctx).With(zap.String("method", LogMethodDelete), zap.String("key", key))
	if err := c.client.Del(ctx, key).Err(); err != nil {
		log.Error(ctx, ErrorFailedToDelete, zap.Error(err))
		return fmt.Errorf("%s: %w", ErrorFailedToDelete, err)
	}
	return nil
}

func (c *RedisCache) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("%s: %w", ErrorFailedToClose, err)
	}
	return nil
}
