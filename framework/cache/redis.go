package cache

// Purpose: To provide a Redis-backed CacheStore driver for GoStack's distributed deployments.
// Philosophy: The "Zero External Bloat" principle still applies — Redis is only introduced when
// the developer explicitly chooses horizontal scale. The Memory driver remains the default for
// local development. This driver plugs seamlessly into the same contract.CacheStore interface,
// so all Generic wrapper functions (Get[T], Put[T], Remember[T]) work without modification.
// Architecture:
// RedisStore wraps the official `go-redis/v9` client. All operations are context-aware,
// using Go's standard `context.Background()` as the execution context.
// Choice:
// We use `go-redis/v9` — the official, maintained Redis client for Go — because it correctly
// handles modern Redis 7.x protocol extensions and provides native context propagation.
// Implementation:
// - Values are serialized to JSON before storing in Redis to guarantee cross-process compatibility.
// - TTL is passed directly to the Redis `SET EX` command, enabling atomic key expiry.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements contract.CacheStore backed by a Redis server.
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisStore creates a new RedisStore connected to the given Redis URL.
// addr is in the format "host:port" (e.g. "localhost:6379").
func NewRedisStore(addr, password string, db int) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisStore{
		client: rdb,
		ctx:    context.Background(),
	}
}

// Get retrieves a value from Redis and deserializes it.
func (r *RedisStore) Get(key string) (any, bool) {
	val, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	var result any
	if err := json.Unmarshal(val, &result); err != nil {
		return nil, false
	}
	return result, true
}

// Put stores a value in Redis, serialized as JSON, with an expiry TTL.
func (r *RedisStore) Put(key string, val any, ttl time.Duration) {
	data, err := json.Marshal(val)
	if err != nil {
		return
	}
	r.client.Set(r.ctx, key, data, ttl)
}

// Forget removes a key from the Redis cache.
func (r *RedisStore) Forget(key string) {
	r.client.Del(r.ctx, key)
}

// Flush deletes all keys from the current Redis database.
func (r *RedisStore) Flush() {
	r.client.FlushDB(r.ctx)
}
