package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisCache struct {
	client     *redis.Client
	defaultTTL time.Duration
}

func NewRedisCache(ctx context.Context, cfg *config.Config) (ports.Cache, error) {
	if cfg == nil || cfg.Redis == nil {
		return nil, fmt.Errorf("%w: missing redis config", domain.ErrInvalidParams)
	}
	redCfg := cfg.Redis
	host := strings.TrimSpace(redCfg.Host)
	if host == "" {
		host = "localhost"
	}
	port := redCfg.Port
	if port <= 0 {
		port = 6379
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	defaultTTL := redCfg.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 15 * time.Minute
	}
	opts := &redis.Options{Addr: addr, Password: redCfg.Password, DB: redCfg.DB, DialTimeout: redCfg.ConnectTimeout, ReadTimeout: redCfg.ReadTimeout, WriteTimeout: redCfg.WriteTimeout, PoolSize: redCfg.PoolSize, MinIdleConns: redCfg.MinIdle, ConnMaxIdleTime: redCfg.IdleTimeout, ConnMaxLifetime: redCfg.MaxConnLifetime}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrRedisConnectFailed, err)
	}
	return new(RedisCache{client: client, defaultTTL: defaultTTL}), nil
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodGet), zap.String("key", key))
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		log.Error(ctx, domain.ErrorFailedToGet, zap.Error(err))
		return "", fmt.Errorf("%w: %v", domain.ErrRedisGetFailed, err)
	}
	return value, nil
}

func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	log := logger.Log(ctx).With(zap.String("method", domain.LogMethodSet), zap.String("key", key))
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		log.Error(ctx, domain.ErrorFailedToSet, zap.Error(err))
		return fmt.Errorf("%w: %v", domain.ErrRedisSetFailed, err)
	}
	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		logger.Log(ctx).With(zap.String("method", domain.LogMethodDelete), zap.String("key", key)).Error(ctx, domain.ErrorFailedToDelete, zap.Error(err))
		return fmt.Errorf("%w: %v", domain.ErrRedisDeleteFailed, err)
	}
	return nil
}

func (c *RedisCache) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrRedisCloseFailed, err)
	}
	return nil
}
