package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTaskQueueRunsAndDeduplicates(t *testing.T) {
	queue := NewTaskQueue(2)
	var (
		mu    sync.Mutex
		runs  = map[string]int{}
		total atomic.Int64
	)
	queue.Start(func(ctx context.Context, projectID string) {
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		runs[projectID]++
		mu.Unlock()
		total.Add(1)
	})

	if !queue.Submit("p1") {
		t.Fatal("first submit should be accepted")
	}
	// Rapid repeats while p1 is queued/running collapse into one run.
	for i := 0; i < 5; i++ {
		if queue.Submit("p1") {
			t.Fatal("duplicate submit should be deduplicated")
		}
	}
	if !queue.Submit("p2") {
		t.Fatal("independent project should be accepted")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if total.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !queue.Stop(2 * time.Second) {
		t.Fatal("queue did not stop cleanly")
	}
	mu.Lock()
	defer mu.Unlock()
	if runs["p1"] != 1 || runs["p2"] != 1 {
		t.Fatalf("expected exactly one run per project, got %#v", runs)
	}
	stats := queue.Stats()
	if stats.Processed != 2 {
		t.Fatalf("processed = %d, want 2", stats.Processed)
	}
	if stats.Deduplicated != 5 {
		t.Fatalf("deduplicated = %d, want 5", stats.Deduplicated)
	}
}

func TestTaskQueueResubmitAfterRunSchedulesCatchUp(t *testing.T) {
	queue := NewTaskQueue(1)
	var total atomic.Int64
	queue.Start(func(ctx context.Context, projectID string) {
		total.Add(1)
	})
	if !queue.Submit("p") {
		t.Fatal("submit failed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && total.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if total.Load() != 1 {
		t.Fatalf("first run missing: %d", total.Load())
	}
	// After the run completed the dedup marker is gone; a new submit runs again.
	if !queue.Submit("p") {
		t.Fatal("resubmit after completion should be accepted")
	}
	for time.Now().Before(deadline) && total.Load() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if total.Load() != 2 {
		t.Fatalf("catch-up run missing: %d", total.Load())
	}
	queue.Stop(2 * time.Second)
}

func TestTaskQueueSubmitWithoutStartIsRejected(t *testing.T) {
	queue := NewTaskQueue(1)
	if queue.Submit("p") {
		t.Fatal("submit before Start must be rejected")
	}
	if !queue.Stop(time.Second) {
		t.Fatal("stop on never-started queue should succeed")
	}
}
