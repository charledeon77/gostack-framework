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

func (s *RedisSession) serialise() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.data)
}

// RedisSessionStore implements contract.SessionStore using a Redis client.
type RedisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisSessionStore creates a Redis-backed session store.
func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &RedisSessionStore{client: client, ttl: ttl}
}

// Load retrieves a session from Redis or creates a new one.
func (store *RedisSessionStore) Load(id string) (contract.Session, error) {
	sess := newRedisSession(id)
	if id == "" {
		return sess, nil
	}

	payload, err := store.client.Get(context.Background(), id).Bytes()
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

	return store.client.Set(context.Background(), s.ID(), payload, store.ttl).Err()
}

// Destroy removes a session from Redis.
func (store *RedisSessionStore) Destroy(id string) error {
	return store.client.Del(context.Background(), id).Err()
}
