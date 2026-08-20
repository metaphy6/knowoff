package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient is a thin wrapper around go-redis that exposes only the
// operations the game server needs for room routing and presence.
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

// RegisterRoom maps roomID -> nodeID with an expiry so stale mappings fade.
func (r *RedisClient) RegisterRoom(ctx context.Context, roomID, nodeID string, ttl time.Duration) error {
	key := fmt.Sprintf("room:%s:node", roomID)
	return r.client.Set(ctx, key, nodeID, ttl).Err()
}

// RoomNode returns the node responsible for a room, or "" if none.
func (r *RedisClient) RoomNode(ctx context.Context, roomID string) (string, error) {
	key := fmt.Sprintf("room:%s:node", roomID)
	return r.client.Get(ctx, key).Result()
}

// UnregisterRoom removes the room→node mapping.
func (r *RedisClient) UnregisterRoom(ctx context.Context, roomID string) error {
	key := fmt.Sprintf("room:%s:node", roomID)
	return r.client.Del(ctx, key).Err()
}
