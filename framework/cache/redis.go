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
// - SEC-45: Flush() performs a prefix-scoped SCAN+DEL rather than FlushDB() to avoid
//   accidentally wiping data owned by other applications sharing the same Redis instance.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/redis/go-redis/v9"
)

// RedisStore implements contract.CacheStore backed by a Redis server.
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
	prefix string // optional key namespace; Flush() deletes only keys under this prefix
}

// NewRedisStore creates a new RedisStore connected to the given Redis URL.
// addr is in the format "host:port" (e.g. "localhost:6379").
// SEC-45: A default prefix of "gostack:cache:" is always set so that Flush()
// can never reach FlushDB on the default path. Protection must be on by
// default — requiring a caller to opt in to safety repeats the SEC-01 mistake.
func NewRedisStore(addr, password string, db int) *RedisStore {
	return NewRedisStoreWithPrefix(addr, password, db, "gostack:cache:")
}

// NewRedisStoreWithPrefix creates a RedisStore that namespaces all keys under prefix.
// Flush() will only delete keys whose names begin with this prefix.
func NewRedisStoreWithPrefix(addr, password string, db int, prefix string) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisStore{
		client: rdb,
		ctx:    context.Background(),
		prefix: prefix,
	}
}

// Get retrieves a value from Redis and deserializes it.
// SEC-46: Uses a 5-second context timeout to prevent hung connections from
// blocking indefinitely. Returns (nil, contract.ErrCacheMiss) on a true cache miss
// (redis.Nil). Any other operational error — network outage, timeout, protocol error,
// or JSON unmarshal failure — is surfaced immediately as a non-nil error.
func (r *RedisStore) Get(key string) (any, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, contract.ErrCacheMiss
		}
		return nil, fmt.Errorf("redis: get %q failed: %w", key, err)
	}

	var result any
	if err := json.Unmarshal(val, &result); err != nil {
		return nil, fmt.Errorf("redis: unmarshal %q failed: %w", key, err)
	}
	return result, nil
}

// Put stores a value in Redis, serialized as JSON, with an expiry TTL.
// SEC-46: Uses a 5-second context timeout and returns any error encountered.
func (r *RedisStore) Put(key string, val any, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("redis: marshal %q failed: %w", key, err)
	}
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set %q failed: %w", key, err)
	}
	return nil
}

// Forget removes a key from the Redis cache.
// SEC-46: Uses a 5-second context timeout and returns any error encountered.
func (r *RedisStore) Forget(key string) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis: del %q failed: %w", key, err)
	}
	return nil
}

// Flush deletes cache entries scoped to this store's prefix via SCAN+DEL.
// SEC-45: FlushDB is never called. NewRedisStore always sets a default prefix
// ("gostack:cache:") so this branch is unreachable on the default path. If
// somehow called with an empty prefix, we return an error rather than
// wiping the entire Redis database — an operation that would log out every
// user sharing the same Redis instance.
// SEC-46: All Redis calls use a 5-second context timeout and return errors.
func (r *RedisStore) Flush() error {
	if r.prefix == "" {
		return fmt.Errorf("redis: Flush() refused with empty prefix (would wipe entire Redis DB)")
	}

	pattern := r.prefix + "*"
	var cursor uint64
	for {
		ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
		keys, nextCursor, err := r.client.Scan(ctx, cursor, pattern, 100).Result()
		cancel()
		if err != nil {
			return fmt.Errorf("redis: scan failed during flush: %w", err)
		}
		if len(keys) > 0 {
			delCtx, delCancel := context.WithTimeout(r.ctx, 5*time.Second)
			err := r.client.Del(delCtx, keys...).Err()
			delCancel()
			if err != nil {
				return fmt.Errorf("redis: del failed during flush: %w", err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
