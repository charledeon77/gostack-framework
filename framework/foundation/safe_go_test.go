package foundation

import (
	"sync"
	"testing"
	"time"
)

func TestSafeGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	completed := false
	SafeGo("test-panicking-goroutine", func() {
		defer wg.Done()
		panic("deliberate panic for recovery test")
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		completed = true
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SafeGo panicking goroutine to exit cleanly")
	}

	if !completed {
		t.Errorf("expected SafeGo goroutine to recover and complete")
	}
}
