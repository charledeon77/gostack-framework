/*
Sequence — GoStack Deferred Background Queue Worker Subsystem.

Naming Metaphor: Why Sequence?
The GoStack Background Queue Subsystem was named Sequence because tasks and background jobs
must be queued, orchestrated, and executed in an orderly, deterministic "sequence". Sequence
ensures that time-intensive operations (sending transactional emails, processing video, generating
PDFs) are deferred off the synchronous HTTP request path and dispatched through a resilient sequence
of workers with automated retries and backoffs.

Purpose: To provide an asynchronous job processing system (Sequence) for GoStack.
Philosophy: A robust framework must handle long-running or failure-prone tasks outside
the critical HTTP request-response cycle. By providing a generic, extensible queue
interface, GoStack allows developers to seamlessly switch between local memory
queues (for dev/testing) and distributed systems like Redis/RabbitMQ (for prod),
without changing application logic.
Architecture:
- Job: An interface defining a processable unit of work (from contract.Job).
- Queue: The interface for pushing and executing jobs (from contract.Queue).
- MemoryQueue: A concurrent-safe, in-memory implementation of Queue.
Choice:
We use a simple `MemoryQueue` backed by a buffered channel for the default implementation.
It provides excellent performance and requires zero configuration.
A `sync.WaitGroup` is used in StartWorkers to safely manage worker goroutines.
*/
package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

var (
	ErrQueueClosed = errors.New("queue is closed")
	ErrQueueFull   = errors.New("queue is full")
)

// memoryJobEnvelope wraps a job and its attempt count for in-memory processing.
type memoryJobEnvelope struct {
	job      contract.Job
	attempts int
}

type memoryFailedJob struct {
	id       string
	job      contract.Job
	payload  string
	attempts int
	errorMsg string
	failedAt time.Time
}

type memoryDelayedItem struct {
	env *memoryJobEnvelope
	due time.Time
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
	failSeq       uint64
	delayedItems  []*memoryDelayedItem
	delayedWake   chan struct{}
}

// NewMemoryQueue creates a new MemoryQueue with the specified buffer capacity.
// D8: Uses a single background scheduler goroutine instead of spawning a goroutine per delayed job.
func NewMemoryQueue(capacity int) *MemoryQueue {
	q := &MemoryQueue{
		jobs:         make(chan *memoryJobEnvelope, capacity),
		failedJobs:   make([]memoryFailedJob, 0),
		delayedItems: make([]*memoryDelayedItem, 0),
		delayedWake:  make(chan struct{}, 1),
	}
	go q.runDelayedScheduler()
	return q
}

// pushEnvelope pushes a job envelope to the internal channel.
// F-173: Uses non-blocking select so holding RLock never deadlocks with Close().
func (q *MemoryQueue) pushEnvelope(env *memoryJobEnvelope) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return ErrQueueClosed
	}

	select {
	case q.jobs <- env:
		return nil
	default:
		return ErrQueueFull
	}
}

// Push adds a job to the back of the queue immediately.
func (q *MemoryQueue) Push(job contract.Job) error {
	return q.pushEnvelope(&memoryJobEnvelope{job: job, attempts: 0})
}

func backoffFor(attempts int, factor time.Duration) time.Duration {
	if factor <= 0 {
		factor = time.Second
	}
	if attempts > 10 {
		attempts = 10
	}
	if attempts < 0 {
		attempts = 0
	}
	return factor * time.Duration(1 << attempts)
}

func (q *MemoryQueue) recordFailure(job contract.Job, attempts int, payload string, reason string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.failedJobs) >= 1000 {
		q.failedJobs = q.failedJobs[1:]
	}
	q.failSeq++
	id := fmt.Sprintf("mem-%d", q.failSeq)
	if payload == "" && job != nil {
		if b, err := json.Marshal(job); err == nil {
			payload = string(b)
		}
	}
	q.failedJobs = append(q.failedJobs, memoryFailedJob{
		id:       id,
		job:      job,
		payload:  payload,
		attempts: attempts,
		errorMsg: reason,
		failedAt: time.Now(),
	})
	return id
}

// PushDelayed dispatches a job to the queue, delaying execution by a set duration.
// D8: Enqueues into the scheduler queue guarded by mutex, avoiding unbounded goroutine growth.
func (q *MemoryQueue) PushDelayed(job contract.Job, delay time.Duration) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrQueueClosed
	}
	item := &memoryDelayedItem{
		env: &memoryJobEnvelope{job: job, attempts: 0},
		due: time.Now().Add(delay),
	}
	q.delayedItems = append(q.delayedItems, item)
	q.mu.Unlock()

	select {
	case q.delayedWake <- struct{}{}:
	default:
	}
	return nil
}

func (q *MemoryQueue) runDelayedScheduler() {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return
		}
		now := time.Now()
		var nextWait time.Duration = time.Second
		var remaining []*memoryDelayedItem
		var due []*memoryDelayedItem

		for _, item := range q.delayedItems {
			if !now.Before(item.due) {
				due = append(due, item)
			} else {
				remaining = append(remaining, item)
				diff := item.due.Sub(now)
				if diff < nextWait {
					nextWait = diff
				}
			}
		}
		q.delayedItems = remaining
		q.mu.Unlock()

		for _, item := range due {
			if pushErr := q.pushEnvelope(item.env); pushErr != nil {
				q.recordFailure(item.env.job, item.env.attempts, "", fmt.Sprintf("delayed push failed: %v", pushErr))
			}
		}

		select {
		case <-time.After(nextWait):
		case <-q.delayedWake:
		}
	}
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
	select {
	case q.delayedWake <- struct{}{}:
	default:
	}
	q.mu.Unlock()

	q.wg.Wait()
	return nil
}

// worker is the internal loop that continuously pops and executes jobs with panic isolation.
func (q *MemoryQueue) worker(id int) {
	defer q.wg.Done()

	for env := range q.jobs {
		func(e *memoryJobEnvelope) {
			if e == nil || e.job == nil {
				return
			}
			jobName := e.job.Name()
			var runErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						// D32: Convert panic to runErr so it follows unified retry & failure logic
						runErr = fmt.Errorf("panic: %v", r)
					}
				}()
				runErr = e.job.Handle()
			}()

			if runErr != nil {
				e.attempts++
				if retryable, ok := e.job.(contract.Retryable); ok && e.attempts < retryable.MaxAttempts() {
					// Exponential backoff with clamping (D2)
					factor := q.BackoffFactor
					if factor == 0 {
						factor = time.Second
					}
					backoff := backoffFor(e.attempts, factor)
					fmt.Printf("[Sequence Worker %d] Job %s execution failed: %v. Retrying in %v (attempt %d/%d)...\n",
						id, jobName, runErr, backoff, e.attempts, retryable.MaxAttempts())

					// D8: Schedule retry through the scheduler rather than leaking sleeping goroutines
					q.mu.Lock()
					if q.closed {
						q.mu.Unlock()
						q.recordFailure(e.job, e.attempts, "", "queue closed during retry schedule")
					} else {
						q.delayedItems = append(q.delayedItems, &memoryDelayedItem{
							env: e,
							due: time.Now().Add(backoff),
						})
						q.mu.Unlock()
						select {
						case q.delayedWake <- struct{}{}:
						default:
						}
					}
				} else {
					fmt.Printf("[Sequence Worker %d] Job %s execution failed: %v\n", id, jobName, runErr)
					// C23: Shared failure recording with full payload and attempts
					q.recordFailure(e.job, e.attempts, "", runErr.Error())
				}
			}
		}(env)
	}
}

// GetStats retrieves overview counters of the queue state.
func (q *MemoryQueue) GetStats() (contract.QueueStats, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return contract.QueueStats{
		Driver:  "memory",
		Pending: int64(len(q.jobs)),
		Delayed: int64(len(q.delayedItems)),
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
			Payload:  f.payload,
			Attempts: f.attempts,
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
