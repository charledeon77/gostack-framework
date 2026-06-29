package cache

import (
	"testing"
	"time"
)

type mockUser struct {
	ID   int
	Name string
}

func TestMemoryStore_BasicOperations(t *testing.T) {
	store := NewMemoryStore()

	// Put
	store.Put("key1", "value1", 0)

	// Get
	val, found := store.Get("key1")
	if !found || val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	// Forget
	store.Forget("key1")
	_, found = store.Get("key1")
	if found {
		t.Errorf("Expected key1 to be forgotten")
	}

	// Flush
	store.Put("key2", "value2", 0)
	store.Flush()
	_, found = store.Get("key2")
	if found {
		t.Errorf("Expected cache to be flushed")
	}
}

func TestMemoryStore_Expiration(t *testing.T) {
	store := NewMemoryStore()

	store.Put("short_lived", "data", 10*time.Millisecond)

	_, found := store.Get("short_lived")
	if !found {
		t.Errorf("Expected item to be found immediately")
	}

	time.Sleep(20 * time.Millisecond)

	_, found = store.Get("short_lived")
	if found {
		t.Errorf("Expected item to be expired")
	}
}

func TestGenericCacheAPI(t *testing.T) {
	store := NewMemoryStore()

	user := mockUser{ID: 1, Name: "Alice"}
	
	// Test Generic Put
	Put(store, "user_1", user, 0)

	// Test Generic Get (Type Safe)
	cachedUser, found := Get[mockUser](store, "user_1")
	if !found {
		t.Errorf("Expected to find user")
	}
	if cachedUser.Name != "Alice" {
		t.Errorf("Expected Alice, got %v", cachedUser.Name)
	}

	// Test Type mismatch (asking for int when struct is stored)
	_, foundAsInt := Get[int](store, "user_1")
	if foundAsInt {
		t.Errorf("Expected false when type assertion fails")
	}

	// Test Remember
	callCount := 0
	val, err := Remember(store, "lazy_key", 0, func() (string, error) {
		callCount++
		return "computed_value", nil
	})
	if err != nil || val != "computed_value" || callCount != 1 {
		t.Errorf("Remember failed on first call")
	}

	// Second call should hit cache and not increment callCount
	val, err = Remember(store, "lazy_key", 0, func() (string, error) {
		callCount++
		return "new_value", nil
	})
	if err != nil || val != "computed_value" || callCount != 1 {
		t.Errorf("Remember failed to use cached value")
	}
}
