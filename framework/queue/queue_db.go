/*
Purpose:
Implements a database-backed queue driver for GoStack's Sequence system.
Allows applications to persist jobs across server restarts and process them
reliably with multiple worker instances without race conditions.

Philosophy:
The MemoryQueue is perfect for development. Production applications need jobs
that survive restarts and can be distributed across multiple workers. The
DatabaseQueue stores jobs in a SQL table with pessimistic row locking
(SELECT ... FOR UPDATE) to ensure each job is processed exactly once.

Architecture:
Jobs are serialised to JSON and stored in a `jobs` table. Workers poll the
table using atomic reserve-then-process logic. Failed jobs are tracked with
attempt counts and moved to the failed_jobs table after exhausting retries.

Required migrations:
  CREATE TABLE jobs (
      id           INTEGER PRIMARY KEY AUTO_INCREMENT,
      queue        VARCHAR(128) NOT NULL DEFAULT 'default',
      payload      TEXT         NOT NULL,
      attempts     INTEGER      NOT NULL DEFAULT 0,
      reserved_at  INTEGER      NULL,
      available_at INTEGER      NOT NULL,
      created_at   INTEGER      NOT NULL
  );

  CREATE TABLE failed_jobs (
      id         INTEGER PRIMARY KEY AUTO_INCREMENT,
      queue      VARCHAR(128) NOT NULL,
      payload    TEXT         NOT NULL,
      attempts   INTEGER      NOT NULL,
      error      TEXT         NOT NULL,
      failed_at  INTEGER      NOT NULL
  );

Implementation:
- DatabaseQueue: Implements contract.Queue using a *sql.DB backend.
- Push / PushDelayed: Insert jobs into the jobs table.
- StartWorkers: Spawns goroutines that poll, reserve, execute, and acknowledge jobs.
- GetFailedJobs / RetryJob / DeleteFailedJob: Failed job management.
*/
package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// dbJobEnvelope is the JSON structure persisted to the jobs table.
type dbJobEnvelope struct {
	Name    string `json:"name"`
	Payload []byte `json:"payload"`
}

// DatabaseQueue is a SQL-backed implementation of contract.Queue.
// It is safe for use across multiple worker instances.
type DatabaseQueue struct {
	db          *sql.DB
	table       string       // jobs table name
	failedTable string       // failed_jobs table name
	queue       string       // logical queue name (allows multiple queues in one table)
	pollInterval time.Duration
	registry    map[string]func() contract.Job // job name → constructor
	mu          sync.RWMutex
	wg          sync.WaitGroup
	stop        chan struct{}
}

// NewDatabaseQueue creates a database-backed queue driver.
// table defaults to "jobs", failedTable defaults to "failed_jobs", queue defaults to "default".
func NewDatabaseQueue(db *sql.DB, table, failedTable, queueName string, pollInterval time.Duration) *DatabaseQueue {
	if table == "" {
		table = "jobs"
	}
	if failedTable == "" {
		failedTable = "failed_jobs"
	}
	if queueName == "" {
		queueName = "default"
	}
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	return &DatabaseQueue{
		db:           db,
		table:        table,
		failedTable:  failedTable,
		queue:        queueName,
		pollInterval: pollInterval,
		registry:     make(map[string]func() contract.Job),
		stop:         make(chan struct{}),
	}
}

// Register maps a job name to a constructor so workers can re-hydrate jobs
// from their serialised JSON payload.
//
// Example:
//
//	dbq.Register("SendWelcomeEmail", func() contract.Job { return &SendWelcomeEmail{} })
func (q *DatabaseQueue) Register(name string, constructor func() contract.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.registry[name] = constructor
}

// Push inserts a job into the database queue immediately.
func (q *DatabaseQueue) Push(job contract.Job) error {
	return q.pushAt(job, time.Now())
}

// PushDelayed inserts a job into the database queue, available after the delay.
func (q *DatabaseQueue) PushDelayed(job contract.Job, delay time.Duration) error {
	return q.pushAt(job, time.Now().Add(delay))
}

func (q *DatabaseQueue) pushAt(job contract.Job, availableAt time.Time) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("[DatabaseQueue] failed to serialise job: %w", err)
	}

	env := dbJobEnvelope{Name: job.Name(), Payload: payload}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("[DatabaseQueue] failed to serialise envelope: %w", err)
	}

	now := time.Now().Unix()
	_, err = q.db.Exec(
		fmt.Sprintf(`INSERT INTO %s (queue, payload, attempts, available_at, created_at) VALUES (?, ?, 0, ?, ?)`, q.table),
		q.queue, string(envBytes), availableAt.Unix(), now,
	)
	return err
}

// StartWorkers launches n goroutines that each poll the database for available jobs.
func (q *DatabaseQueue) StartWorkers(workers int) {
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

// Close signals all workers to stop and waits for them to finish.
func (q *DatabaseQueue) Close() error {
	close(q.stop)
	q.wg.Wait()
	return nil
}

// worker polls the database table, reserves a job atomically, and executes it.
func (q *DatabaseQueue) worker(id int) {
	defer q.wg.Done()

	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.stop:
			return
		case <-ticker.C:
			if err := q.processNext(id); err != nil {
				// Non-fatal — will retry on the next tick.
				fmt.Printf("[DatabaseQueue Worker %d] poll error: %v\n", id, err)
			}
		}
	}
}

// processNext atomically reserves and processes one available job.
func (q *DatabaseQueue) processNext(workerID int) error {
	tx, err := q.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().Unix()

	// Attempt to reserve one unreserved, available job.
	row := tx.QueryRow(
		fmt.Sprintf(`SELECT id, payload, attempts FROM %s
		             WHERE queue = ? AND available_at <= ? AND reserved_at IS NULL
		             ORDER BY id ASC LIMIT 1 FOR UPDATE`, q.table),
		q.queue, now,
	)

	var jobID int64
	var payload string
	var attempts int
	if err = row.Scan(&jobID, &payload, &attempts); err != nil {
		if err == sql.ErrNoRows {
			_ = tx.Rollback()
			return nil // Nothing to process right now.
		}
		return err
	}

	// Mark as reserved.
	if _, err = tx.Exec(
		fmt.Sprintf(`UPDATE %s SET reserved_at = ?, attempts = attempts + 1 WHERE id = ?`, q.table),
		now, jobID,
	); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	// Deserialise and run the job.
	var env dbJobEnvelope
	if err = json.Unmarshal([]byte(payload), &env); err != nil {
		q.deleteJob(jobID)
		return fmt.Errorf("corrupt job payload (id=%d): %w", jobID, err)
	}

	q.mu.RLock()
	constructor, ok := q.registry[env.Name]
	q.mu.RUnlock()

	if !ok {
		q.deleteJob(jobID)
		return fmt.Errorf("no registered handler for job %q (id=%d)", env.Name, jobID)
	}

	job := constructor()
	if err = json.Unmarshal(env.Payload, job); err != nil {
		q.deleteJob(jobID)
		return fmt.Errorf("failed to hydrate job %q (id=%d): %w", env.Name, jobID, err)
	}

	runErr := job.Handle()
	if runErr != nil {
		attempts++
		if retryable, isRetryable := job.(contract.Retryable); isRetryable && attempts < retryable.MaxAttempts() {
			// Put it back as unreserved for a retry.
			_, _ = q.db.Exec(
				fmt.Sprintf(`UPDATE %s SET reserved_at = NULL, available_at = ? WHERE id = ?`, q.table),
				time.Now().Add(time.Duration(attempts)*10*time.Second).Unix(), jobID,
			)
			fmt.Printf("[DatabaseQueue Worker %d] Job %s failed (attempt %d/%d), retrying: %v\n",
				workerID, env.Name, attempts, retryable.MaxAttempts(), runErr)
			return nil
		}

		// Permanently failed — move to failed_jobs.
		_, _ = q.db.Exec(
			fmt.Sprintf(`INSERT INTO %s (queue, payload, attempts, error, failed_at) VALUES (?, ?, ?, ?, ?)`, q.failedTable),
			q.queue, payload, attempts, runErr.Error(), time.Now().Unix(),
		)
		fmt.Printf("[DatabaseQueue Worker %d] Job %s permanently failed: %v\n", workerID, env.Name, runErr)
	}

	q.deleteJob(jobID)
	return nil
}

func (q *DatabaseQueue) deleteJob(id int64) {
	_, _ = q.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", q.table), id)
}

// GetStats returns overview counters for this queue.
func (q *DatabaseQueue) GetStats() (contract.QueueStats, error) {
	var pending, failed int64

	row := q.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE queue = ? AND reserved_at IS NULL`, q.table), q.queue)
	_ = row.Scan(&pending)

	row = q.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE queue = ?`, q.failedTable), q.queue)
	_ = row.Scan(&failed)

	return contract.QueueStats{
		Driver:  "database",
		Pending: pending,
		Delayed: 0,
		Failed:  failed,
	}, nil
}

// GetFailedJobs lists all permanently failed jobs for this queue.
func (q *DatabaseQueue) GetFailedJobs() ([]contract.FailedJob, error) {
	rows, err := q.db.Query(
		fmt.Sprintf(`SELECT id, payload, attempts, error, failed_at FROM %s WHERE queue = ? ORDER BY id DESC`, q.failedTable),
		q.queue,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []contract.FailedJob
	for rows.Next() {
		var id int64
		var payload, errMsg string
		var attempts int
		var failedAt int64

		if err := rows.Scan(&id, &payload, &attempts, &errMsg, &failedAt); err != nil {
			continue
		}

		var env dbJobEnvelope
		_ = json.Unmarshal([]byte(payload), &env)

		jobs = append(jobs, contract.FailedJob{
			ID:       fmt.Sprintf("%d", id),
			Name:     env.Name,
			Payload:  payload,
			Attempts: attempts,
			Error:    errMsg,
			FailedAt: time.Unix(failedAt, 0),
		})
	}
	return jobs, rows.Err()
}

// RetryJob re-queues a failed job by its numeric ID.
func (q *DatabaseQueue) RetryJob(id string) error {
	row := q.db.QueryRow(
		fmt.Sprintf(`SELECT payload FROM %s WHERE id = ?`, q.failedTable), id)
	var payload string
	if err := row.Scan(&payload); err != nil {
		return fmt.Errorf("failed job not found: %s", id)
	}

	now := time.Now().Unix()
	_, err := q.db.Exec(
		fmt.Sprintf(`INSERT INTO %s (queue, payload, attempts, available_at, created_at) VALUES (?, ?, 0, ?, ?)`, q.table),
		q.queue, payload, now, now,
	)
	if err != nil {
		return err
	}

	_, _ = q.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, q.failedTable), id)
	return nil
}

// DeleteFailedJob permanently removes a failed job by ID.
func (q *DatabaseQueue) DeleteFailedJob(id string) error {
	res, err := q.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, q.failedTable), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("failed job not found: %s", id)
	}
	return nil
}
