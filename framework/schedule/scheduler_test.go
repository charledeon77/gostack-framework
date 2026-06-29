package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_Execution(t *testing.T) {
	s := New()

	var counter int32

	// Schedule a fast job
	s.Every(10*time.Millisecond, func() error {
		atomic.AddInt32(&counter, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Run in background
	go s.Run(ctx)

	// Wait long enough for the job to run a few times (e.g. ~5 times)
	time.Sleep(55 * time.Millisecond)

	// Cancel context to stop scheduler
	cancel()
	
	// Wait to ensure graceful shutdown
	s.Stop()

	count := atomic.LoadInt32(&counter)
	
	// It should have executed at least 2 times.
	if count < 2 {
		t.Errorf("Expected counter to be at least 2, got %d", count)
	}
}
