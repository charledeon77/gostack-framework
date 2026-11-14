package cache

import (
	"errors"
	"testing"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

type mockUser struct {
	ID   int
	Name string
}

func TestMemoryStore_BasicOperations(t *testing.T) {
	store := NewMemoryStore()

	// Put
	if err := store.Put("key1", "value1", 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	val, err := store.Get("key1")
	if err != nil || val != "value1" {
		t.Errorf("Expected 'value1', got val=%v, err=%v", val, err)
	}

	// Forget
	if err := store.Forget("key1"); err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	_, err = store.Get("key1")
	if !errors.Is(err, contract.ErrCacheMiss) {
		t.Errorf("Expected ErrCacheMiss after Forget, got %v", err)
	}

	// Flush
	if err := store.Put("key2", "value2", 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_, err = store.Get("key2")
	if !errors.Is(err, contract.ErrCacheMiss) {
		t.Errorf("Expected ErrCacheMiss after Flush, got %v", err)
	}
}

func TestMemoryStore_Expiration(t *testing.T) {
	store := NewMemoryStore()

	if err := store.Put("short_lived", "data", 10*time.Millisecond); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	_, err := store.Get("short_lived")
	if err != nil {
		t.Errorf("Expected item to be found immediately, got %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	_, err = store.Get("short_lived")
	if !errors.Is(err, contract.ErrCacheMiss) {
		t.Errorf("Expected ErrCacheMiss on expired item, got %v", err)
	}
}

func TestGenericCacheAPI(t *testing.T) {
	store := NewMemoryStore()

	user := mockUser{ID: 1, Name: "Alice"}

	// Test Generic Put
	if err := Put(store, "user_1", user, 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test Generic Get (Type Safe)
	cachedUser, err := Get[mockUser](store, "user_1")
	if err != nil {
		t.Fatalf("Expected to find user, got error %v", err)
	}
	if cachedUser.Name != "Alice" {
		t.Errorf("Expected Alice, got %v", cachedUser.Name)
	}

	// Test Type mismatch (asking for int when struct is stored)
	_, err = Get[int](store, "user_1")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("Expected ErrTypeMismatch when type assertion fails, got %v", err)
	}

	// Test Remember
	callCount := 0
	val, err := Remember(store, "lazy_key", 0, func() (string, error) {
		callCount++
		return "computed_value", nil
	})
	if err != nil || val != "computed_value" || callCount != 1 {
		t.Errorf("Remember failed on first call: val=%v, err=%v, count=%d", val, err, callCount)
	}

	// Second call should hit cache and not increment callCount
	val, err = Remember(store, "lazy_key", 0, func() (string, error) {
		callCount++
		return "new_value", nil
	})
	if err != nil || val != "computed_value" || callCount != 1 {
		t.Errorf("Remember failed to use cached value: val=%v, err=%v, count=%d", val, err, callCount)
	}
}

func TestRedisStore_SEC46_SurfacesErrors(t *testing.T) {
	// Point RedisStore to an unreachable port to verify connection error is surfaced, not swallowed
	redisStore := NewRedisStore("127.0.0.1:59999", "", 0)

	_, err := redisStore.Get("test_key")
	if err == nil {
		t.Fatal("expected error from unreachable Redis store, got nil")
	}
	if errors.Is(err, contract.ErrCacheMiss) {
		t.Fatal("unreachable Redis should return connection error, NOT ErrCacheMiss")
	}

	err = redisStore.Put("test_key", "val", time.Minute)
	if err == nil {
		t.Fatal("expected error from unreachable Redis Put, got nil")
	}

	// Flush with empty prefix must return error
	unprefixedStore := NewRedisStoreWithPrefix("127.0.0.1:59999", "", 0, "")
	if err := unprefixedStore.Flush(); err == nil {
		t.Fatal("expected error on Flush with empty prefix, got nil")
	}
}
