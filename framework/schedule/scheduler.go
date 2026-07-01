package schedule

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Purpose: Native background task scheduling engine (Planner) for GoStack.
// Philosophy: Modern web apps shouldn't need an external cron daemon for periodic
// tasks. This engine uses Go's concurrent select loops — zero external dependencies.
//
// New in this version:
//   - Timezone support: jobs can run in any IANA time zone.
//   - Overlap prevention: WithoutOverlapping() skips a tick if the prior run is
//     still executing, preventing pile-ups under slow tasks.
//   - RunOnce: optional flag to fire the task immediately on boot before the
//     first interval/cron tick.

// Task represents a function that can be scheduled.
type Task func() error

// Job represents a registered task with its scheduling constraints.
type Job struct {
	Interval       time.Duration
	CronExpr       string
	Task           Task
	timezone       *time.Location // nil means UTC/local
	withoutOverlap bool
	runOnBoot      bool
	running        int32 // accessed via sync/atomic; 1 while task is executing
}

// Timezone sets the IANA time zone for the job (e.g. "America/New_York").
// Falls back to UTC if the zone is invalid.
func (j *Job) Timezone(zone string) *Job {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	j.timezone = loc
	return j
}

// WithoutOverlapping prevents a new execution from starting if the previous
// invocation of this job is still running. Safe to use on long-running tasks.
func (j *Job) WithoutOverlapping() *Job {
	j.withoutOverlap = true
	return j
}

// RunOnBoot fires the task once immediately when the scheduler starts, before
// the first scheduled tick. Useful for warming up caches or running initial checks.
func (j *Job) RunOnBoot() *Job {
	j.runOnBoot = true
	return j
}

// inLocation returns the supplied time converted to the job's configured timezone.
// If no timezone is set, the original time is returned unchanged.
func (j *Job) inLocation(t time.Time) time.Time {
	if j.timezone != nil {
		return t.In(j.timezone)
	}
	return t
}

// tryRun executes the task, honouring WithoutOverlapping if set.
// It returns false (and skips execution) when a previous run is still in progress.
func (j *Job) tryRun(id int) {
	if j.withoutOverlap {
		if !atomic.CompareAndSwapInt32(&j.running, 0, 1) {
			// Previous run still in flight — skip this tick.
			fmt.Printf("[Scheduler] Job %d skipped — previous execution still running.\n", id)
			return
		}
		defer atomic.StoreInt32(&j.running, 0)
	}

	if err := j.Task(); err != nil {
		fmt.Printf("[Scheduler] Error executing job %d: %v\n", id, err)
	}
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
// Returns the *Job so callers can chain .Timezone(), .WithoutOverlapping(), .RunOnBoot().
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

// Cron registers a task using a standard 5-field cron expression.
// Returns the *Job so callers can chain .Timezone(), .WithoutOverlapping(), .RunOnBoot().
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

// Run starts the scheduler. It blocks until the context is canceled.
// Typically called in a goroutine: go scheduler.Run(ctx)
func (s *Scheduler) Run(ctx context.Context) {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	jobsToRun := make([]*Job, len(s.jobs))
	copy(jobsToRun, s.jobs)
	s.mu.Unlock()

	for i, job := range jobsToRun {
		// Fire on-boot tasks immediately before entering the tick loop.
		if job.runOnBoot {
			s.wg.Add(1)
			go func(id int, j *Job) {
				defer s.wg.Done()
				j.tryRun(id)
			}(i, job)
		}

		s.wg.Add(1)
		go s.runJob(s.ctx, i, job)
	}

	<-s.ctx.Done()
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
		// Align the initial check to the next minute boundary in the job's timezone.
		now := job.inLocation(time.Now())
		nextMinute := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextMinute)):
		}

		// Check and run at the start of the first minute boundary.
		if matchCron(job.CronExpr, job.inLocation(time.Now())) {
			job.tryRun(id)
		}

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				if matchCron(job.CronExpr, job.inLocation(t)) {
					job.tryRun(id)
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
				job.tryRun(id)
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
// Supports: * (wildcard), */n (step), a-b (range), a,b,c (list), n (exact).
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
			part = strings.TrimSpace(part)
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

	// Simple exact match
	partVal, err := strconv.Atoi(field)
	return err == nil && partVal == val
}
