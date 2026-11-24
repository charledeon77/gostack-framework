/*
Purpose:
Implements a Redis-backed session store for GoStack applications that need
persistent sessions across server restarts or multi-instance deployments.

Philosophy:
The in-memory store is ideal for development, but production deployments commonly
require shared sessions across multiple instances. This store keeps the same
contract.SessionStore interface, so it is a zero-change drop-in replacement.

Architecture:
Sessions are serialized to JSON and stored in Redis using the session ID as the key.
Each session has a TTL so expired data is automatically removed by Redis.

Implementation:
- RedisSession: in-memory session data, serialized to Redis on Save.
- RedisSessionStore: loads, saves, and destroys session state via Redis.
*/
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/redis/go-redis/v9"
)

// RedisSession is a session implementation backed by Redis.
type RedisSession struct {
	mu   sync.RWMutex
	id   string
	data map[string]any
}

func newRedisSession(id string) *RedisSession {
	return &RedisSession{id: id, data: make(map[string]any)}
}

func (s *RedisSession) ID() string { return s.id }

func (s *RedisSession) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

func (s *RedisSession) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

func (s *RedisSession) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *RedisSession) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]any)
}

func (s *RedisSession) Flash(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data["__flash__"+key] = val
}

func (s *RedisSession) GetFlash(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	flashKey := "__flash__" + key
	val, exists := s.data[flashKey]
	if !exists {
		return nil
	}
	delete(s.data, flashKey)
	return val
}

// All returns a shallow copy of all key-value pairs stored in the session.
func (s *RedisSession) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]any, len(s.data))
	for k, v := range s.data {
		res[k] = v
	}
	return res
}

func (s *RedisSession) serialise() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.data)
}

// RedisSessionStore implements contract.SessionStore using a Redis client.
type RedisSessionStore struct {
	client  *redis.Client
	ttl     time.Duration
	timeout time.Duration
}

// NewRedisSessionStore creates a Redis-backed session store.
func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &RedisSessionStore{client: client, ttl: ttl, timeout: 5 * time.Second}
}

// SetTimeout configures the request deadline for Redis operations (C24).
func (store *RedisSessionStore) SetTimeout(d time.Duration) *RedisSessionStore {
	store.timeout = d
	return store
}

func (store *RedisSessionStore) withTimeout() (context.Context, context.CancelFunc) {
	to := store.timeout
	if to <= 0 {
		to = 5 * time.Second
	}
	return context.WithTimeout(context.Background(), to)
}

// Load retrieves a session from Redis or creates a new one.
func (store *RedisSessionStore) Load(id string) (contract.Session, error) {
	sess := newRedisSession(id)
	if id == "" {
		return sess, nil
	}

	ctx, cancel := store.withTimeout()
	defer cancel()
	payload, err := store.client.Get(ctx, id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return sess, nil
		}
		return nil, fmt.Errorf("redis session store: load failed: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return sess, nil
	}

	sess.data = data
	return sess, nil
}

// Save persists the session into Redis.
func (store *RedisSessionStore) Save(s contract.Session) error {
	sess, ok := s.(*RedisSession)
	if !ok {
		return fmt.Errorf("redis session store: expected *RedisSession, got %T", s)
	}

	payload, err := sess.serialise()
	if err != nil {
		return fmt.Errorf("redis session store: marshal failed: %w", err)
	}

	ctx, cancel := store.withTimeout()
	defer cancel()
	return store.client.Set(ctx, s.ID(), payload, store.ttl).Err()
}

// Exists reports whether the session exists in Redis and is not expired (SEC-16).
// C5: Returns an error on Redis store outage.
func (store *RedisSessionStore) Exists(id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	ctx, cancel := store.withTimeout()
	defer cancel()
	n, err := store.client.Exists(ctx, id).Result()
	if err != nil {
		return false, fmt.Errorf("redis session store: exists check failed: %w", err)
	}
	return n > 0, nil
}

// Destroy removes a session from Redis.
func (store *RedisSessionStore) Destroy(id string) error {
	ctx, cancel := store.withTimeout()
	defer cancel()
	return store.client.Del(ctx, id).Err()
}

// Rotate generates a new ID, migrates data, and deletes the old record in Redis (SEC-04).
func (store *RedisSessionStore) Rotate(oldID string) (contract.Session, error) {
	oldSess, err := store.Load(oldID)
	if err != nil {
		return nil, err
	}

	newID := generateSessionID()
	newSess := newRedisSession(newID)

	for k, v := range oldSess.All() {
		newSess.data[k] = v
	}

	if err := store.Destroy(oldID); err != nil {
		return nil, fmt.Errorf("failed to destroy old session during rotation: %w", err)
	}
	if err := store.Save(newSess); err != nil {
		return nil, err
	}

	return newSess, nil
}
