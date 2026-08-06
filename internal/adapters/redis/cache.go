package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/flexer2006/notes-microservices/internal/config"
)

const (
	defaultHost     = "localhost"
	defaultPort     = 6379
	defaultCacheTTL = 15 * time.Minute
)

var errRedisConfigRequired = errors.New("redis config is required")

type Cache struct {
	client     *redis.Client
	defaultTTL time.Duration
}

func NewRedisCache(ctx context.Context, cfg *config.Config) (*Cache, error) {
	if cfg == nil || cfg.Redis == nil {
		return nil, errRedisConfigRequired
	}

	redCfg := cfg.Redis

	host := strings.TrimSpace(redCfg.Host)
	if host == "" {
		host = defaultHost
	}

	port := redCfg.Port
	if port <= 0 {
		port = defaultPort
	}

	defaultTTL := redCfg.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = defaultCacheTTL
	}

	opts := new(redis.Options{
		Addr:            net.JoinHostPort(host, strconv.Itoa(port)),
		Password:        redCfg.Password,
		DB:              redCfg.DB,
		DialTimeout:     redCfg.ConnectTimeout,
		ReadTimeout:     redCfg.ReadTimeout,
		WriteTimeout:    redCfg.WriteTimeout,
		PoolSize:        redCfg.PoolSize,
		MinIdleConns:    redCfg.MinIdle,
		ConnMaxIdleTime: redCfg.IdleTimeout,
		ConnMaxLifetime: redCfg.MaxConnLifetime,
	})

	client := redis.NewClient(opts)

	pingErr := client.Ping(ctx).Err()
	if pingErr != nil {
		return nil, fmt.Errorf("redis: connect: %w", pingErr)
	}

	return new(Cache{client: client, defaultTTL: defaultTTL}), nil
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}

		return "", fmt.Errorf("redis: get: %w", err)
	}

	return value, nil
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	err := c.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("redis: set: %w", err)
	}

	return nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("redis: delete: %w", err)
	}

	return nil
}

func (c *Cache) Ping(ctx context.Context) error {
	err := c.client.Ping(ctx).Err()
	if err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}

	return nil
}

func (c *Cache) Close() error {
	err := c.client.Close()
	if err != nil {
		return fmt.Errorf("redis: close: %w", err)
	}

	return nil
}
