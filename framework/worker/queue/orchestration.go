package queue

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

var (
	registryMu     sync.RWMutex
	globalRegistry = make(map[string]func() contract.Job)
)

// RegisterJob binds a unique job name to a constructor factory function so that
// serialized jobs (including nested jobs inside JobChain and JobBatch) can be
// accurately reconstructed during queue polling and unmarshaling.
// If the constructor produces a job whose Name() differs from the passed name,
// both identifiers are bound to guarantee resolution consistency.
func RegisterJob(name string, constructor func() contract.Job) {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry[name] = constructor
	if sample := constructor(); sample != nil {
		if sampleName := sample.Name(); sampleName != "" && sampleName != name {
			globalRegistry[sampleName] = constructor
		}
	}
}

// LookupJob retrieves the constructor factory associated with the specified job name.
func LookupJob(name string) (func() contract.Job, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := globalRegistry[name]
	return fn, ok
}

func init() {
	RegisterJob("Sequence.JobChain", func() contract.Job { return &JobChain{} })
	RegisterJob("Sequence.JobBatch", func() contract.Job { return &JobBatch{} })
}

type jobEnvelope struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

type jobListEnvelope struct {
	Jobs []jobEnvelope `json:"jobs"`
}

// JobChain executes a series of jobs sequentially. If one fails, the chain stops.
// F-179: Exported Jobs field with custom MarshalJSON and UnmarshalJSON for interface hydration.
type JobChain struct {
	Jobs []contract.Job `json:"jobs"`
}

// NewChain constructs a new JobChain.
func NewChain(jobs ...contract.Job) *JobChain {
	return &JobChain{Jobs: jobs}
}

// MarshalJSON serializes the JobChain with envelopes preserving job names and payloads.
func (c *JobChain) MarshalJSON() ([]byte, error) {
	envelopes := make([]jobEnvelope, 0, len(c.Jobs))
	for _, job := range c.Jobs {
		if job == nil {
			continue
		}
		raw, err := json.Marshal(job)
		if err != nil {
			return nil, fmt.Errorf("queue: failed to marshal job in chain %s: %w", job.Name(), err)
		}
		envelopes = append(envelopes, jobEnvelope{
			Name:    job.Name(),
			Payload: raw,
		})
	}
	return json.Marshal(jobListEnvelope{Jobs: envelopes})
}

// UnmarshalJSON reconstructs concrete job instances from registered constructors.
func (c *JobChain) UnmarshalJSON(data []byte) error {
	var list jobListEnvelope
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	c.Jobs = make([]contract.Job, 0, len(list.Jobs))
	for _, env := range list.Jobs {
		constructor, ok := LookupJob(env.Name)
		if !ok {
			return fmt.Errorf("queue: cannot unmarshal job in chain: job name %q is not registered in job registry", env.Name)
		}
		job := constructor()
		if len(env.Payload) > 0 && string(env.Payload) != "null" {
			if err := json.Unmarshal(env.Payload, job); err != nil {
				return fmt.Errorf("queue: failed to unmarshal job payload for %q: %w", env.Name, err)
			}
		}
		c.Jobs = append(c.Jobs, job)
	}
	return nil
}

// Handle executes the chain of jobs.
// F-180: Protects each step with panic recovery so one panic doesn't kill worker.
func (c *JobChain) Handle() error {
	for _, job := range c.Jobs {
		var stepErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					stepErr = fmt.Errorf("job %s panicked: %v", job.Name(), r)
				}
			}()
			stepErr = job.Handle()
		}()
		if stepErr != nil {
			return stepErr
		}
	}
	return nil
}

// Name returns the job name.
func (c *JobChain) Name() string {
	return "Sequence.JobChain"
}

// JobBatch represents a collection of jobs to run concurrently.
// F-179: Exported Jobs field so json.Marshal serializes the batch.
type JobBatch struct {
	Jobs      []contract.Job  `json:"jobs"`
	onFail    func(err error) `json:"-"`
	onSuccess func()          `json:"-"`
}

// NewBatch constructs a JobBatch.
func NewBatch(jobs ...contract.Job) *JobBatch {
	return &JobBatch{Jobs: jobs}
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

	for _, job := range b.Jobs {
		wg.Add(1)
		go func(j contract.Job) {
			defer wg.Done()
			// SEC-33: Recover from panics inside batch job goroutines so a single
			// panicking job does not kill the process or leave the WaitGroup stuck.
			defer func() {
				if r := recover(); r != nil {
					once.Do(func() {
						firstErr = fmt.Errorf("job panicked: %v", r)
						if b.onFail != nil {
							b.onFail(firstErr)
						}
					})
				}
			}()
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

// MarshalJSON serializes the JobBatch with envelopes preserving job names and payloads.
func (b *JobBatch) MarshalJSON() ([]byte, error) {
	envelopes := make([]jobEnvelope, 0, len(b.Jobs))
	for _, job := range b.Jobs {
		if job == nil {
			continue
		}
		raw, err := json.Marshal(job)
		if err != nil {
			return nil, fmt.Errorf("queue: failed to marshal job in batch %s: %w", job.Name(), err)
		}
		envelopes = append(envelopes, jobEnvelope{
			Name:    job.Name(),
			Payload: raw,
		})
	}
	return json.Marshal(jobListEnvelope{Jobs: envelopes})
}

// UnmarshalJSON reconstructs concrete job instances from registered constructors.
func (b *JobBatch) UnmarshalJSON(data []byte) error {
	var list jobListEnvelope
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	b.Jobs = make([]contract.Job, 0, len(list.Jobs))
	for _, env := range list.Jobs {
		constructor, ok := LookupJob(env.Name)
		if !ok {
			return fmt.Errorf("queue: cannot unmarshal job in batch: job name %q is not registered in job registry", env.Name)
		}
		job := constructor()
		if len(env.Payload) > 0 && string(env.Payload) != "null" {
			if err := json.Unmarshal(env.Payload, job); err != nil {
				return fmt.Errorf("queue: failed to unmarshal job payload for %q: %w", env.Name, err)
			}
		}
		b.Jobs = append(b.Jobs, job)
	}
	return nil
}
