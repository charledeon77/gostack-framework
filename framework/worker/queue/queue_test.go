package queue

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockJob is a simple job used for testing.
type mockJob struct {
	processed int32
	shouldErr bool
	name      string
}

func (m *mockJob) Handle() error {
	if m.shouldErr {
		return errors.New("simulated job error")
	}
	atomic.AddInt32(&m.processed, 1)
	return nil
}

func (m *mockJob) Name() string {
	return m.name
}

func TestMemoryQueue_PushAndProcess(t *testing.T) {
	q := NewMemoryQueue(10)
	q.StartWorkers(2)

	job1 := &mockJob{name: "Job1"}
	job2 := &mockJob{name: "Job2"}

	err := q.Push(job1)
	if err != nil {
		t.Fatalf("Failed to push job1: %v", err)
	}
	err = q.Push(job2)
	if err != nil {
		t.Fatalf("Failed to push job2: %v", err)
	}

	// wait for processing
	time.Sleep(100 * time.Millisecond)

	if job1.processed != 1 {
		t.Errorf("Expected job1 to be processed once, got %d", job1.processed)
	}
	if job2.processed != 1 {
		t.Errorf("Expected job2 to be processed once, got %d", job2.processed)
	}

	err = q.Close()
	if err != nil {
		t.Fatalf("Failed to close queue: %v", err)
	}
}

func TestMemoryQueue_PushDelayed(t *testing.T) {
	q := NewMemoryQueue(10)
	q.StartWorkers(1)

	job := &mockJob{name: "DelayedJob"}
	err := q.PushDelayed(job, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to push delayed job: %v", err)
	}

	// initially not processed
	if atomic.LoadInt32(&job.processed) != 0 {
		t.Errorf("Job processed too early")
	}

	// wait longer than delay to allow goroutine worker scheduling
	time.Sleep(400 * time.Millisecond)

	if atomic.LoadInt32(&job.processed) != 1 {
		t.Errorf("Expected delayed job to be processed once, got %d", atomic.LoadInt32(&job.processed))
	}

	q.Close()
}

// retryableMockJob is a mock job that implements contract.Retryable and always fails.
type retryableMockJob struct {
	processedCount int32
	maxAttempts    int
}

func (r *retryableMockJob) Handle() error {
	atomic.AddInt32(&r.processedCount, 1)
	return errors.New("simulated retryable job failure")
}

func (r *retryableMockJob) Name() string {
	return "RetryableMockJob"
}

func (r *retryableMockJob) MaxAttempts() int {
	return r.maxAttempts
}

func TestMemoryQueue_Retry(t *testing.T) {
	q := NewMemoryQueue(10)
	q.BackoffFactor = 5 * time.Millisecond // fast backoff for testing
	q.StartWorkers(2)

	job := &retryableMockJob{
		maxAttempts: 3,
	}

	err := q.Push(job)
	if err != nil {
		t.Fatalf("Failed to push job: %v", err)
	}

	// Wait for retries to run (1st run + 2 retries = 3 total runs)
	// Backoff durations:
	// - 1st failure: 2 * 5ms = 10ms
	// - 2nd failure: 4 * 5ms = 20ms
	// Total wait needs to cover ~50-100ms.
	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt32(&job.processedCount)
	if count != 3 {
		t.Errorf("Expected job to be processed exactly 3 times (1 initial + 2 retries), got %d", count)
	}

	q.Close()
}
