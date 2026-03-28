package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

// Creates a new Redis client with RESP3 protocol enabled.
// addr should be "host:port", password can be empty.
func NewRedisCache(ctx context.Context, addr string, password string, db int) (*RedisCache, error) {
	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Protocol: 3, // RESP3
	}

	client := redis.NewClient(opts)

	// Verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("unable to ping redis: %w", err)
	}

	return &RedisCache{client: client}, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Set stores a value with an expiration.
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a string value.
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Client returns the underlying redis client (useful for Asynq later).
func (c *RedisCache) Client() *redis.Client {
	return c.client
}
