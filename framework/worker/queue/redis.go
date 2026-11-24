package queue

// Purpose: To provide a Redis-backed queue driver for distributed GoStack deployments.
// Philosophy: The MemoryQueue is excellent for single-instance workloads and local development.
// However, in a multi-server, load-balanced production deployment, jobs pushed to Server A's
// memory queue are invisible to workers running on Server B. RedisQueue solves this by providing
// a shared, durable job bus that all application instances can read from and write to.
// Architecture:
// Uses Redis Lists (`LPUSH` / `BRPOP`) as a reliable FIFO job queue. Jobs are serialized to JSON
// before dispatch and deserialized by workers. A worker pool goroutine continuously calls BRPOP
// (blocking pop), which means it sleeps efficiently at the Redis level until a job arrives.
// Choice:
// Redis Lists are the simplest, most battle-tested primitive for task queues — they are used
// identically by Sidekiq (Ruby) and Laravel Horizon (PHP). We chose BRPOP (blocking) over
// polling to eliminate busy-waiting and reduce CPU usage on idle workers.
// Implementation:
// - Push: serializes the job struct to JSON and pushes it to the queue list with LPUSH.
// - PushDelayed: uses Redis ZADD on a sorted set keyed by Unix timestamp for delayed execution.
// - StartWorkers: spawns N goroutines that all BRPOP from the same list, auto-distributing load.

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/foundation"
)

const redisQueueKey = "gostack:queue:default"
const redisDelayedKey = "gostack:queue:delayed"
const redisFailedKey = "gostack:queue:failed"

// jobPayload is the envelope used to serialize/deserialize jobs stored in Redis.
type jobPayload struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Data     json.RawMessage `json:"data"`
	Attempts int             `json:"attempts"`
	Error    string          `json:"error,omitempty"`
}

// RedisQueue implements contract.Queue using a Redis List as the backing store.
type RedisQueue struct {
	client        *redis.Client
	ctx           context.Context
	cancel        context.CancelFunc
	registry      map[string]func() contract.Job
	BackoffFactor time.Duration // Custom backoff multiplier (defaults to time.Second)
}

// NewRedisQueue creates a new Redis-backed queue.
// registry maps job names to constructor functions so deserialization can produce
// the correct concrete Job type.
func NewRedisQueue(addr, password string, db int, registry map[string]func() contract.Job) *RedisQueue {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithCancel(context.Background())
	for name, constructor := range registry {
		RegisterJob(name, constructor)
	}
	return &RedisQueue{
		client:   rdb,
		ctx:      ctx,
		cancel:   cancel,
		registry: registry,
	}
}

// pushPayload encodes the payload and pushes it to the head of the Redis list.
func (q *RedisQueue) pushPayload(payload jobPayload) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("redisqueue: failed to encode payload: %w", err)
	}
	return q.client.LPush(q.ctx, redisQueueKey, encoded).Err()
}

// pushDelayedPayload encodes the payload and ZAdds it with the correct delayed execution score.
func (q *RedisQueue) pushDelayedPayload(payload jobPayload, delay time.Duration) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("redisqueue: failed to encode delayed payload: %w", err)
	}
	score := float64(time.Now().Add(delay).UnixMilli())
	return q.client.ZAdd(q.ctx, redisDelayedKey, redis.Z{Score: score, Member: encoded}).Err()
}

// Push serializes the job and pushes it to the head of the Redis list.
func (q *RedisQueue) Push(job contract.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("redisqueue: failed to serialize job: %w", err)
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	id := fmt.Sprintf("job-%d-%06d", time.Now().UnixNano(), n.Int64())
	payload := jobPayload{ID: id, Name: job.Name(), Data: data, Attempts: 0}
	return q.pushPayload(payload)
}

// PushDelayed adds a job to a Redis Sorted Set, scored by its future execution timestamp.
func (q *RedisQueue) PushDelayed(job contract.Job, delay time.Duration) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("redisqueue: failed to serialize delayed job: %w", err)
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	id := fmt.Sprintf("job-%d-%06d", time.Now().UnixNano(), n.Int64())
	payload := jobPayload{ID: id, Name: job.Name(), Data: data, Attempts: 0}
	return q.pushDelayedPayload(payload, delay)
}

// StartWorkers spawns N worker goroutines that each block on BRPOP, waiting for jobs.
func (q *RedisQueue) StartWorkers(workers int) {
	for i := 0; i < workers; i++ {
		foundation.SafeGo("redis-queue-worker", q.runWorker)
	}
	// Also run a scheduler goroutine that migrates due delayed jobs to the main queue.
	foundation.SafeGo("redis-delayed-scheduler", q.runDelayedScheduler)
}

func (q *RedisQueue) runWorker() {
	for {
		if q.ctx.Err() != nil {
			return
		}
		// BRPOP blocks for up to 5 seconds waiting for a job, then retries.
		result, err := q.client.BRPop(q.ctx, 5*time.Second, redisQueueKey).Result()
		if err != nil {
			if q.ctx.Err() != nil {
				return
			}
			// Transient error or timeout — slight sleep to prevent CPU spinning (F-154)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if len(result) < 2 {
			continue
		}
		raw := result[1]
		var payload jobPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			fmt.Printf("[Sequence Worker] Failed to decode job payload: %v\n", err)
			continue
		}
		constructor, exists := q.registry[payload.Name]
		if !exists {
			fmt.Printf("[Sequence Worker] Unknown job type: %s\n", payload.Name)
			continue
		}
		job := constructor()
		if err := json.Unmarshal(payload.Data, job); err != nil {
			fmt.Printf("[Sequence Worker] Failed to unmarshal job data: %v\n", err)
			continue
		}

		// Isolate individual job panics: a panicking job is treated as a failure
		// and dead-lettered rather than killing the worker goroutine.
		var handleErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					handleErr = fmt.Errorf("job %q panicked: %v", payload.Name, r)
				}
			}()
			handleErr = job.Handle()
		}()

		if handleErr != nil {
			payload.Attempts++
			payload.Error = handleErr.Error()
			if retryable, ok := job.(contract.Retryable); ok && payload.Attempts < retryable.MaxAttempts() {
				// Exponential backoff with clamping (D2)
				factor := q.BackoffFactor
				if factor == 0 {
					factor = time.Second
				}
				backoff := backoffFor(payload.Attempts, factor)
				fmt.Printf("[Sequence Worker] Job %s failed: %v. Retrying in %v (attempt %d/%d)...\n",
					payload.Name, handleErr, backoff, payload.Attempts, retryable.MaxAttempts())
				
				if retryErr := q.pushDelayedPayload(payload, backoff); retryErr != nil {
					fmt.Printf("[Sequence Worker] Failed to queue retry for job %s: %v\n", payload.Name, retryErr)
				}
			} else {
				fmt.Printf("[Sequence Worker] Job %s failed: %v. Storing in dead-letter list.\n", payload.Name, handleErr)
				failedEncoded, marshalErr := json.Marshal(payload)
				if marshalErr == nil {
					if dlErr := q.client.LPush(q.ctx, redisFailedKey, failedEncoded).Err(); dlErr != nil {
						fmt.Printf("[Sequence Worker] Failed to write dead letter: %v\n", dlErr)
					}
				} else {
					if dlErr := q.client.LPush(q.ctx, redisFailedKey, raw).Err(); dlErr != nil {
						fmt.Printf("[Sequence Worker] Failed to write raw dead letter: %v\n", dlErr)
					}
				}
			}
		}
	}
}

// runDelayedScheduler periodically checks for jobs whose delay has elapsed and
// moves them into the main queue for processing.
func (q *RedisQueue) runDelayedScheduler() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			now := float64(time.Now().UnixMilli())
			// Fetch all delayed jobs scored up to now.
			jobs, err := q.client.ZRangeByScore(q.ctx, redisDelayedKey, &redis.ZRangeBy{
				Min: "-inf",
				Max: fmt.Sprintf("%f", now),
			}).Result()
			if err != nil || len(jobs) == 0 {
				continue
			}
			for _, job := range jobs {
				// B6: only remove from delayed sorted set if LPush succeeded
				if err := q.client.LPush(q.ctx, redisQueueKey, job).Err(); err == nil {
					if remErr := q.client.ZRem(q.ctx, redisDelayedKey, job).Err(); remErr != nil {
						// D4: log failure to remove from delayed set
						fmt.Printf("[RedisQueue] Warning: failed to remove delayed job from %s: %v (job may run twice)\n", redisDelayedKey, remErr)
					}
				}
			}
		}
	}
}

// Close gracefully terminates the RedisQueue workers and releases resources.
func (q *RedisQueue) Close() error {
	if q.cancel != nil {
		q.cancel()
	}
	if q.client != nil {
		return q.client.Close()
	}
	return nil
}

// GetStats retrieves overview counters of the queue state.
func (q *RedisQueue) GetStats() (contract.QueueStats, error) {
	pending, err := q.client.LLen(q.ctx, redisQueueKey).Result()
	if err != nil {
		return contract.QueueStats{}, err
	}

	delayed, err := q.client.ZCard(q.ctx, redisDelayedKey).Result()
	if err != nil {
		return contract.QueueStats{}, err
	}

	failed, err := q.client.LLen(q.ctx, redisFailedKey).Result()
	if err != nil {
		return contract.QueueStats{}, err
	}

	return contract.QueueStats{
		Driver:  "redis",
		Pending: pending,
		Delayed: delayed,
		Failed:  failed,
	}, nil
}

// GetFailedJobs lists failed jobs in the Redis queue.
func (q *RedisQueue) GetFailedJobs() ([]contract.FailedJob, error) {
	// F-160: cap LRANGE to 1000 to prevent unbounded memory consumption.
	rawJobs, err := q.client.LRange(q.ctx, redisFailedKey, 0, 999).Result()
	if err != nil {
		return nil, err
	}

	failedJobs := make([]contract.FailedJob, 0, len(rawJobs))
	for _, r := range rawJobs {
		var payload jobPayload
		if err := json.Unmarshal([]byte(r), &payload); err == nil {
			failedJobs = append(failedJobs, contract.FailedJob{
				ID:       payload.ID,
				Name:     payload.Name,
				Payload:  string(payload.Data),
				Attempts: payload.Attempts,
				Error:    payload.Error,
				FailedAt: time.Now(), 
			})
		}
	}
	return failedJobs, nil
}

// RetryJob re-queues a failed job by its ID.
func (q *RedisQueue) RetryJob(id string) error {
	// F-160: cap LRANGE to 1000 to prevent unbounded memory consumption.
	rawJobs, err := q.client.LRange(q.ctx, redisFailedKey, 0, 999).Result()
	if err != nil {
		return err
	}

	for _, r := range rawJobs {
		var payload jobPayload
		if err := json.Unmarshal([]byte(r), &payload); err == nil {
			if payload.ID == id {
				// 1. Remove from failed list
				_, err = q.client.LRem(q.ctx, redisFailedKey, 1, r).Result()
				if err != nil {
					return err
				}

				// 2. Reset attempts and error
				payload.Attempts = 0
				payload.Error = ""

				// 3. Push to pending queue
				return q.pushPayload(payload)
			}
		}
	}
	return fmt.Errorf("job not found: %s", id)
}

// DeleteFailedJob permanently removes a failed job by its ID.
func (q *RedisQueue) DeleteFailedJob(id string) error {
	// F-160: cap LRANGE to 1000 to prevent unbounded memory consumption.
	rawJobs, err := q.client.LRange(q.ctx, redisFailedKey, 0, 999).Result()
	if err != nil {
		return err
	}

	for _, r := range rawJobs {
		var payload jobPayload
		if err := json.Unmarshal([]byte(r), &payload); err == nil {
			if payload.ID == id {
				// Remove from failed list
				_, err = q.client.LRem(q.ctx, redisFailedKey, 1, r).Result()
				return err
			}
		}
	}
	return fmt.Errorf("job not found: %s", id)
}
