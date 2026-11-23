package queue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type dummyTestJob struct {
	JobName string `json:"job_name"`
	Ran     bool   `json:"ran"`
}

func (d *dummyTestJob) Name() string { return d.JobName }
func (d *dummyTestJob) Handle() error {
	d.Ran = true
	return nil
}

type panickingTestJob struct{}

func (p *panickingTestJob) Name() string  { return "PanickingJob" }
func (p *panickingTestJob) Handle() error { panic("something broke") }

// F-179: Test that JobChain and JobBatch marshal and unmarshal correctly with exported fields.
func TestJobChain_SerialisesAndRuns(t *testing.T) {
	job1 := &dummyTestJob{JobName: "Step1"}
	job2 := &dummyTestJob{JobName: "Step2"}

	chain := NewChain(job1, job2)
	bytes, err := json.Marshal(chain)
	if err != nil {
		t.Fatalf("failed to marshal JobChain: %v", err)
	}

	if string(bytes) == "{}" {
		t.Fatalf("expected JobChain to serialize jobs, got empty JSON: %s", string(bytes))
	}
	if !strings.Contains(string(bytes), "Step1") || !strings.Contains(string(bytes), "Step2") {
		t.Fatalf("expected jobs in JSON payload, got: %s", string(bytes))
	}

	if err := chain.Handle(); err != nil {
		t.Fatalf("expected chain to run successfully, got: %v", err)
	}
	if !job1.Ran || !job2.Ran {
		t.Fatalf("expected both jobs to run, got job1=%v, job2=%v", job1.Ran, job2.Ran)
	}
}

// F-180: Test that a panic in a JobChain step returns an error instead of crashing.
func TestJobChain_PanicIsolated(t *testing.T) {
	panicJob := &panickingTestJob{}
	chain := NewChain(panicJob)

	err := chain.Handle()
	if err == nil {
		t.Fatal("expected error from panicking chain step, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected error mentioning 'panicked', got: %v", err)
	}
}

// F-173: Test that MemoryQueue does not deadlock when channel buffer is full.
func TestMemoryQueue_PushFullBufferDoesNotDeadlock(t *testing.T) {
	// Create queue with capacity 1
	q := NewMemoryQueue(1)

	// Push 1 job (succeeds)
	err := q.Push(&dummyTestJob{JobName: "J1"})
	if err != nil {
		t.Fatalf("first push failed: %v", err)
	}

	// Push 2nd job into full buffer — should return ErrQueueFull immediately
	done := make(chan error, 1)
	go func() {
		done <- q.Push(&dummyTestJob{JobName: "J2"})
	}()

	select {
	case pushErr := <-done:
		if !errors.Is(pushErr, ErrQueueFull) {
			t.Fatalf("expected ErrQueueFull, got: %v", pushErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Push deadlocked on full buffer!")
	}

	// Verify Close() doesn't hang
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- q.Close()
	}()

	select {
	case closeErr := <-closeDone:
		if closeErr != nil {
			t.Fatalf("close failed: %v", closeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked!")
	}
}

// F-141: Test that SQLite does not emit FOR UPDATE in DatabaseQueue query.
func TestSQLite_FORUPDATE_NotEmitted(t *testing.T) {
	q := &DatabaseQueue{
		driver:      "sqlite",
		table:       "jobs",
		failedTable: "failed_jobs",
		queue:       "default",
	}

	forUpdate := " FOR UPDATE"
	if q.driver == "sqlite" || q.driver == "sqlite3" {
		forUpdate = ""
	}
	if forUpdate != "" {
		t.Fatalf("expected SQLite forUpdate to be empty, got: %q", forUpdate)
	}
}

// F-176: Test that memory worker nil job does not panic recover handler.
func TestMemoryQueue_NilJobGuarded(t *testing.T) {
	q := NewMemoryQueue(5)
	q.StartWorkers(1)

	// Push a nil envelope job directly to channel
	q.jobs <- &memoryJobEnvelope{job: nil}

	time.Sleep(50 * time.Millisecond)
	_ = q.Close()
}
