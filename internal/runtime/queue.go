package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TaskQueue is an in-process, per-project deduplicating task queue. Channel
// events submit projects; background workers drain each project's ready
// tasks. Repeated submissions while a project is queued or running collapse
// into one run, and a submission during a run schedules a catch-up pass so
// newly created tasks are not missed.
type TaskQueue struct {
	workers int

	mu      sync.Mutex
	pending map[string]struct{}
	work    chan string
	runner  func(ctx context.Context, projectID string)
	started bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	running   atomic.Int64
	processed atomic.Int64
	dropped   atomic.Int64
}

// QueueStats is the observable state of a TaskQueue.
type QueueStats struct {
	QueuedProjects int64 `json:"queued_projects"`
	Running        int64 `json:"running"`
	Processed      int64 `json:"processed"`
	Deduplicated   int64 `json:"deduplicated"`
	Workers        int   `json:"workers"`
}

// NewTaskQueue creates a queue with the given worker count (minimum 1).
// Call Start before submitting.
func NewTaskQueue(workers int) *TaskQueue {
	if workers <= 0 {
		workers = 1
	}
	return &TaskQueue{
		workers: workers,
		pending: map[string]struct{}{},
		work:    make(chan string, 256),
	}
}

// Start launches the worker goroutines with the given project runner.
func (q *TaskQueue) Start(runner func(ctx context.Context, projectID string)) {
	if runner == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		return
	}
	q.started = true
	q.runner = runner
	q.ctx, q.cancel = context.WithCancel(context.Background())
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
}

func (q *TaskQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case projectID := <-q.work:
			q.running.Add(1)
			runner := q.runnerSnapshot()
			if runner != nil {
				runner(q.ctx, projectID)
			}
			q.running.Add(-1)
			q.processed.Add(1)
			q.markDone(projectID)
		}
	}
}

func (q *TaskQueue) runnerSnapshot() func(ctx context.Context, projectID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.runner
}

// Submit queues a project run. It returns false when the project is already
// queued (deduplicated), when the queue buffer is full, or after Stop.
func (q *TaskQueue) Submit(projectID string) bool {
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		return false
	}
	if _, queued := q.pending[projectID]; queued {
		q.mu.Unlock()
		q.dropped.Add(1)
		return false
	}
	q.pending[projectID] = struct{}{}
	q.mu.Unlock()

	select {
	case q.work <- projectID:
		return true
	default:
		// Buffer full: unmark and report as not queued.
		q.mu.Lock()
		delete(q.pending, projectID)
		q.mu.Unlock()
		q.dropped.Add(1)
		return false
	}
}

// markDone removes the pending marker after a run so later submissions
// trigger a fresh pass.
func (q *TaskQueue) markDone(projectID string) {
	q.mu.Lock()
	delete(q.pending, projectID)
	q.mu.Unlock()
}

// Stats returns the current queue counters. QueuedProjects includes the
// project currently being run by each worker.
func (q *TaskQueue) Stats() QueueStats {
	q.mu.Lock()
	pending := int64(len(q.pending))
	q.mu.Unlock()
	return QueueStats{
		QueuedProjects: pending,
		Running:        q.running.Load(),
		Processed:      q.processed.Load(),
		Deduplicated:   q.dropped.Load(),
		Workers:        q.workers,
	}
}

// Stop cancels workers and waits up to timeout for in-flight runs to
// finish. It reports whether all workers exited cleanly.
func (q *TaskQueue) Stop(timeout time.Duration) bool {
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		return true
	}
	q.started = false
	q.mu.Unlock()

	q.cancel()
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
