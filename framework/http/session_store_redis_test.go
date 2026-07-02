package http

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisSessionStore(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mini.Close()

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()

	store := NewRedisSessionStore(client, time.Minute)

	sess, err := store.Load("sess-redis")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	sess.Set("name", "redis")
	if err := store.Save(sess); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := store.Load("sess-redis")
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if got := reloaded.Get("name"); got != "redis" {
		t.Fatalf("expected reloaded value to be redis, got %v", got)
	}

	if err := store.Destroy("sess-redis"); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	fresh, err := store.Load("sess-redis")
	if err != nil {
		t.Fatalf("load after destroy failed: %v", err)
	}
	if got := fresh.Get("name"); got != nil {
		t.Fatalf("expected fresh session to be empty, got %v", got)
	}
}
