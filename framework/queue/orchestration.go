package queue

import (
	"github.com/charledeon77/gostack-framework/framework/contract"
	"sync"
)

// JobChain executes a series of jobs sequentially. If one fails, the chain stops.
type JobChain struct {
	jobs []contract.Job
}

// NewChain constructs a new JobChain.
func NewChain(jobs ...contract.Job) *JobChain {
	return &JobChain{jobs: jobs}
}

// Handle executes the chain of jobs.
func (c *JobChain) Handle() error {
	for _, job := range c.jobs {
		if err := job.Handle(); err != nil {
			return err
		}
	}
	return nil
}

// Name returns the job name.
func (c *JobChain) Name() string {
	return "Sequence.JobChain"
}

// JobBatch represents a collection of jobs to run concurrently.
type JobBatch struct {
	jobs      []contract.Job
	onFail    func(err error)
	onSuccess func()
}

// NewBatch constructs a JobBatch.
func NewBatch(jobs ...contract.Job) *JobBatch {
	return &JobBatch{jobs: jobs}
}

// OnSuccess sets the callback to execute when all jobs complete successfully.
func (b *JobBatch) OnSuccess(fn func()) *JobBatch {
	b.onSuccess = fn
	return b
}

// OnFail sets the callback to execute if any job fails.
func (b *JobBatch) OnFail(fn func(err error)) *JobBatch {
	b.onFail = fn
	return b
}

// Handle runs the batch.
func (b *JobBatch) Handle() error {
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

	for _, job := range b.jobs {
		wg.Add(1)
		go func(j contract.Job) {
			defer wg.Done()
			if err := j.Handle(); err != nil {
				once.Do(func() {
					firstErr = err
					if b.onFail != nil {
						b.onFail(err)
					}
				})
			}
		}(job)
	}

	wg.Wait()
	if firstErr == nil && b.onSuccess != nil {
		b.onSuccess()
	}
	return firstErr
}

// Name returns the job name.
func (b *JobBatch) Name() string {
	return "Sequence.JobBatch"
}
