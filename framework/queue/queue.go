package queue

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charledeon77/gostack/framework/contract"
)

// Purpose: To provide an asynchronous job processing system (Sequence) for GoStack.
// Philosophy: A robust framework must handle long-running or failure-prone tasks outside
// the critical HTTP request-response cycle. By providing a generic, extensible queue
// interface, GoStack allows developers to seamlessly switch between local memory
// queues (for dev/testing) and distributed systems like Redis/RabbitMQ (for prod),
// without changing application logic.
// Architecture:
// - Job: An interface defining a processable unit of work (from contract.Job).
// - Queue: The interface for pushing and executing jobs (from contract.Queue).
// - MemoryQueue: A concurrent-safe, in-memory implementation of Queue.
// Choice:
// We use a simple `MemoryQueue` backed by a buffered channel for the default implementation.
// It provides excellent performance and requires zero configuration.
// A `sync.WaitGroup` is used in StartWorkers to safely manage worker goroutines.

var (
	ErrQueueClosed = errors.New("queue is closed")
)

// memoryJobEnvelope wraps a job and its attempt count for in-memory processing.
type memoryJobEnvelope struct {
	job      contract.Job
	attempts int
}

type memoryFailedJob struct {
	id       string
	job      contract.Job
	errorMsg string
	failedAt time.Time
}

// MemoryQueue is an in-memory, channel-backed implementation of the contract.Queue interface.
// It is ideal for local development, testing, and single-instance deployments.
type MemoryQueue struct {
	jobs          chan *memoryJobEnvelope
	wg            sync.WaitGroup
	mu            sync.RWMutex
	closed        bool
	BackoffFactor time.Duration // Custom backoff multiplier (defaults to time.Second)
	failedJobs    []memoryFailedJob
}

// NewMemoryQueue creates a new MemoryQueue with the specified buffer capacity.
func NewMemoryQueue(capacity int) *MemoryQueue {
	return &MemoryQueue{
		jobs:       make(chan *memoryJobEnvelope, capacity),
		failedJobs: make([]memoryFailedJob, 0),
	}
}

// pushEnvelope pushes a job envelope to the internal channel.
func (q *MemoryQueue) pushEnvelope(env *memoryJobEnvelope) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return ErrQueueClosed
	}

	q.jobs <- env
	return nil
}

// Push adds a job to the back of the queue immediately.
func (q *MemoryQueue) Push(job contract.Job) error {
	return q.pushEnvelope(&memoryJobEnvelope{job: job, attempts: 0})
}

// PushDelayed dispatches a job to the queue, delaying execution by a set duration.
// In this simple memory queue, we spawn a goroutine to wait, then push.
func (q *MemoryQueue) PushDelayed(job contract.Job, delay time.Duration) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	q.mu.RUnlock()

	go func() {
		time.Sleep(delay)
		_ = q.Push(job) // ignore error as it might be closed during wait
	}()
	return nil
}

// StartWorkers starts a pool of background worker goroutines to process jobs.
func (q *MemoryQueue) StartWorkers(workers int) {
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

// Close gracefully shuts down the queue and waits for workers to finish.
func (q *MemoryQueue) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.jobs) // workers will drain the channel and exit
	q.mu.Unlock()

	q.wg.Wait()
	return nil
}

// worker is the internal loop that continuously pops and executes jobs.
func (q *MemoryQueue) worker(id int) {
	defer q.wg.Done()

	for env := range q.jobs {
		err := env.job.Handle()
		if err != nil {
			env.attempts++
			if retryable, ok := env.job.(contract.Retryable); ok && env.attempts < retryable.MaxAttempts() {
				// Exponential backoff: factor * 2^attempts
				factor := q.BackoffFactor
				if factor == 0 {
					factor = time.Second
				}
				backoff := factor * time.Duration(1 << env.attempts)
				fmt.Printf("[Sequence Worker %d] Job %s execution failed: %v. Retrying in %v (attempt %d/%d)...\n",
					id, env.job.Name(), err, backoff, env.attempts, retryable.MaxAttempts())
				
				go func(e *memoryJobEnvelope, d time.Duration) {
					time.Sleep(d)
					_ = q.pushEnvelope(e)
				}(env, backoff)
			} else {
				fmt.Printf("[Sequence Worker %d] Job %s execution failed: %v\n", id, env.job.Name(), err)
				q.mu.Lock()
				q.failedJobs = append(q.failedJobs, memoryFailedJob{
					id:       fmt.Sprintf("mem-%d", time.Now().UnixNano()),
					job:      env.job,
					errorMsg: err.Error(),
					failedAt: time.Now(),
				})
				q.mu.Unlock()
			}
		}
	}
}

// GetStats retrieves overview counters of the queue state.
func (q *MemoryQueue) GetStats() (contract.QueueStats, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return contract.QueueStats{
		Driver:  "memory",
		Pending: int64(len(q.jobs)),
		Delayed: 0,
		Failed:  int64(len(q.failedJobs)),
	}, nil
}

// GetFailedJobs lists failed jobs in the memory queue.
func (q *MemoryQueue) GetFailedJobs() ([]contract.FailedJob, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	jobs := make([]contract.FailedJob, len(q.failedJobs))
	for i, f := range q.failedJobs {
		jobs[i] = contract.FailedJob{
			ID:       f.id,
			Name:     f.job.Name(),
			Payload:  "{}", 
			Attempts: 1, 
			Error:    f.errorMsg,
			FailedAt: f.failedAt,
		}
	}
	return jobs, nil
}

// RetryJob re-queues a failed job by its ID.
func (q *MemoryQueue) RetryJob(id string) error {
	q.mu.Lock()
	var jobToRetry contract.Job
	found := false
	for i, f := range q.failedJobs {
		if f.id == id {
			jobToRetry = f.job
			q.failedJobs = append(q.failedJobs[:i], q.failedJobs[i+1:]...)
			found = true
			break
		}
	}
	q.mu.Unlock()

	if !found {
		return fmt.Errorf("job not found: %s", id)
	}

	return q.Push(jobToRetry)
}

// DeleteFailedJob permanently removes a failed job by its ID.
func (q *MemoryQueue) DeleteFailedJob(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, f := range q.failedJobs {
		if f.id == id {
			q.failedJobs = append(q.failedJobs[:i], q.failedJobs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("job not found: %s", id)
}
