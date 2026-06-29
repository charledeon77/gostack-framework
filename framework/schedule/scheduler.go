package schedule

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Purpose: To provide a native, background task scheduling engine (Planner) for GoStack.
// Philosophy: Modern web applications shouldn't require external Cron daemons for simple
// periodic tasks (like cleaning up sessions or sending daily reports). By building a
// native scheduler using Go's concurrent `select` loops, we maintain the "Zero External Bloat"
// philosophy while delivering full Cron-like capabilities.
// Architecture:
// - Job: Represents a single schedulable function.
// - Scheduler: The engine holding the registry of jobs and managing their tick loops.
// Choice:
// We use `time.Ticker` for fixed-interval execution. We also include a `sync.WaitGroup`
// so the application can gracefully shut down the scheduler without interrupting in-flight jobs.
// Implementation:
// - `Every(duration)`: Registers a job to run at fixed intervals.
// - `Cron(expr)`: Registers a job using standard 5-field cron syntax.
// - `Run(ctx)`: Boots the scheduler goroutines.

// Task represents a function that can be scheduled.
type Task func() error

// Job represents a registered task with its scheduling constraints.
type Job struct {
	Interval time.Duration
	CronExpr string
	Task     Task
}

// Scheduler manages the execution of periodic background jobs.
type Scheduler struct {
	jobs   []*Job
	mu     sync.Mutex
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new task scheduler.
func New() *Scheduler {
	return &Scheduler{
		jobs: make([]*Job, 0),
	}
}

// Every registers a task to run repeatedly at the given interval.
func (s *Scheduler) Every(interval time.Duration, task Task) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := &Job{
		Interval: interval,
		Task:     task,
	}
	s.jobs = append(s.jobs, job)
	return job
}

// Cron registers a task to run repeatedly according to a 5-field cron expression.
func (s *Scheduler) Cron(expr string, task Task) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := &Job{
		CronExpr: expr,
		Task:     task,
	}
	s.jobs = append(s.jobs, job)
	return job
}

// Run starts the scheduler in the background. It blocks until the context is canceled.
// Typically, you call this in a goroutine: `go scheduler.Run(ctx)`
func (s *Scheduler) Run(ctx context.Context) {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	jobsToRun := make([]*Job, len(s.jobs))
	copy(jobsToRun, s.jobs)
	s.mu.Unlock()

	for i, job := range jobsToRun {
		s.wg.Add(1)
		go s.runJob(s.ctx, i, job)
	}

	// Block until context is done
	<-s.ctx.Done()

	// Wait for any running jobs to finish gracefully
	s.wg.Wait()
}

// Stop sends the cancellation signal to all jobs and waits for them to finish.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) runJob(ctx context.Context, id int, job *Job) {
	defer s.wg.Done()

	if job.CronExpr != "" {
		// Align initial check to the minute boundary
		now := time.Now()
		nextMinute := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return
		case <-time.After(nextMinute.Sub(now)):
		}

		// Run immediately at the start of the first minute boundary
		if matchCron(job.CronExpr, time.Now()) {
			if err := job.Task(); err != nil {
				fmt.Printf("[Scheduler] Error executing cron job %d: %v\n", id, err)
			}
		}

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if matchCron(job.CronExpr, time.Now()) {
					if err := job.Task(); err != nil {
						fmt.Printf("[Scheduler] Error executing cron job %d: %v\n", id, err)
					}
				}
			}
		}
	} else {
		ticker := time.NewTicker(job.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := job.Task()
				if err != nil {
					fmt.Printf("[Scheduler] Error executing job %d: %v\n", id, err)
				}
			}
		}
	}
}

// matchCron parses and matches the cron expression with the given time.
func matchCron(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	return matchField(minute, t.Minute()) &&
		matchField(hour, t.Hour()) &&
		matchField(dom, t.Day()) &&
		matchField(month, int(t.Month())) &&
		matchField(dow, int(t.Weekday()))
}

// matchField evaluates a single cron field expression.
func matchField(field string, val int) bool {
	if field == "*" {
		return true
	}

	// Handle steps e.g. */5
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil {
			return false
		}
		return val%step == 0
	}

	// Handle lists e.g. 1,2,5
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if partVal, err := strconv.Atoi(part); err == nil && partVal == val {
				return true
			}
		}
		return false
	}

	// Handle ranges e.g. 1-5
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				return val >= start && val <= end
			}
		}
		return false
	}

	// Simple match
	partVal, err := strconv.Atoi(field)
	return err == nil && partVal == val
}
