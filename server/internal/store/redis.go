package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient owns the runtime Redis connection, health check and admin limiter.
// V2 room and queue state belongs to the in-process TextManager.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a Redis client from address, password, and DB index.
func NewRedisClient(addr, password string, db int) *RedisClient {
	return &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

// Ping verifies connectivity.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close closes the underlying client.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// AllowIntent implements a sliding-window per-account intent rate limit.
func (r *RedisClient) AllowIntent(ctx context.Context, accountID string, window time.Duration, max int) (bool, error) {
	if accountID == "" || r.client == nil {
		return true, nil
	}
	key := fmt.Sprintf("rate:%s:intents", accountID)
	now := float64(time.Now().UnixNano()) / 1e9
	cutoff := now - window.Seconds()

	pipe := r.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%f", cutoff))
	countCmd := pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: now, Member: now})
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}
	return int(countCmd.Val()) < max, nil
}
