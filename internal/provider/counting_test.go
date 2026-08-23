package provider

import (
	"context"
	"sync"
	"testing"
)

func TestUsageTrackerAggregatesByModelAndStage(t *testing.T) {
	tracker := NewUsageTracker()
	tracker.Record(ChatResponse{Model: "glm-5.3", Usage: map[string]int{"input_tokens": 100, "output_tokens": 50}},
		map[string]string{"stage": "executor.generic.action"})
	tracker.Record(ChatResponse{Model: "glm-5.3", Usage: map[string]int{"input_tokens": 30, "output_tokens": 20}},
		map[string]string{"stage": "planner"})
	tracker.Record(ChatResponse{Model: "glm-5.3", Usage: map[string]int{"input_tokens": 10, "output_tokens": 5}},
		nil)

	snapshot := tracker.Snapshot()
	if snapshot.Totals.Calls != 3 || snapshot.Totals.InputTokens != 140 || snapshot.Totals.OutputTokens != 75 {
		t.Fatalf("unexpected totals: %+v", snapshot.Totals)
	}
	if snapshot.Totals.TotalTokens != 215 {
		t.Fatalf("total tokens = %d, want 215", snapshot.Totals.TotalTokens)
	}
	model := snapshot.ByModel["glm-5.3"]
	if model.Calls != 3 || model.TotalTokens != 215 {
		t.Fatalf("unexpected model counts: %+v", model)
	}
	if snapshot.ByStage["executor.generic.action"].TotalTokens != 150 {
		t.Fatalf("unexpected executor stage counts: %+v", snapshot.ByStage)
	}
	if snapshot.ByStage["unclassified"].TotalTokens != 15 {
		t.Fatalf("unclassified stage missing: %+v", snapshot.ByStage)
	}
}

func TestCountingLLMProviderRecordsUsage(t *testing.T) {
	tracker := NewUsageTracker()
	inner := ChatFunc(func(ctx context.Context, req ChatRequest) (ChatResponse, error) {
		return ChatResponse{Model: req.Model, Text: "{}", Usage: map[string]int{"input_tokens": 7, "output_tokens": 3}}, nil
	})
	counted := WrapWithUsageTracker(inner, tracker)
	if _, err := counted.Chat(context.Background(), ChatRequest{Model: "m", Metadata: map[string]string{"stage": "planner"}}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	snapshot := tracker.Snapshot()
	if snapshot.Totals.Calls != 1 || snapshot.Totals.TotalTokens != 10 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestUsageTrackerConcurrentRecords(t *testing.T) {
	tracker := NewUsageTracker()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Record(ChatResponse{Model: "m", Usage: map[string]int{"input_tokens": 2, "output_tokens": 1}}, nil)
		}()
	}
	wg.Wait()
	snapshot := tracker.Snapshot()
	if snapshot.Totals.Calls != 50 || snapshot.Totals.TotalTokens != 150 {
		t.Fatalf("unexpected totals after concurrent writes: %+v", snapshot.Totals)
	}
}

func TestUsageSnapshotSummary(t *testing.T) {
	snapshot := UsageSnapshot{
		Totals:  UsageCounts{Calls: 2, InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		ByStage: map[string]UsageCounts{"planner": {TotalTokens: 15}},
	}
	summary := snapshot.Summary()
	if summary != "token usage: input=10 output=5 total=15 calls=2 [planner=15]" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}
