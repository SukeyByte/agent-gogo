package provider

import (
	"context"
	"sort"
	"strconv"
	"sync"
)

// UsageCounts is the aggregated token usage for one bucket (total, model, or
// pipeline stage).
type UsageCounts struct {
	Calls        int `json:"calls"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// UsageSnapshot is a point-in-time copy of everything the tracker recorded.
type UsageSnapshot struct {
	Totals  UsageCounts            `json:"totals"`
	ByModel map[string]UsageCounts `json:"by_model"`
	ByStage map[string]UsageCounts `json:"by_stage"`
}

// UsageTracker aggregates token usage reported by LLM providers. It is safe
// for concurrent use and survives provider hot-swaps when it wraps the
// swappable provider from the outside.
type UsageTracker struct {
	mu      sync.Mutex
	totals  UsageCounts
	byModel map[string]UsageCounts
	byStage map[string]UsageCounts
}

func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		byModel: map[string]UsageCounts{},
		byStage: map[string]UsageCounts{},
	}
}

// Record adds one LLM response's usage to the tracker. Metadata keys "stage"
// and the response model are used for the per-bucket breakdowns.
func (t *UsageTracker) Record(resp ChatResponse, metadata map[string]string) {
	if t == nil {
		return
	}
	model := resp.Model
	if model == "" {
		model = "unknown"
	}
	stage := metadata["stage"]
	if stage == "" {
		stage = "unclassified"
	}
	in, out, total := resp.Usage["input_tokens"], resp.Usage["output_tokens"], resp.Usage["total_tokens"]
	if total == 0 {
		total = in + out
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := UsageCounts{Calls: 1, InputTokens: in, OutputTokens: out, TotalTokens: total}
	t.totals = addCounts(t.totals, entry)
	t.byModel[model] = addCounts(t.byModel[model], entry)
	t.byStage[stage] = addCounts(t.byStage[stage], entry)
}

// Snapshot returns a consistent copy of the tracked usage.
func (t *UsageTracker) Snapshot() UsageSnapshot {
	if t == nil {
		return UsageSnapshot{ByModel: map[string]UsageCounts{}, ByStage: map[string]UsageCounts{}}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := UsageSnapshot{
		Totals:  t.totals,
		ByModel: make(map[string]UsageCounts, len(t.byModel)),
		ByStage: make(map[string]UsageCounts, len(t.byStage)),
	}
	for model, counts := range t.byModel {
		snapshot.ByModel[model] = counts
	}
	for stage, counts := range t.byStage {
		snapshot.ByStage[stage] = counts
	}
	return snapshot
}

// Summary renders a compact single-line usage report for logs and the CLI.
func (s UsageSnapshot) Summary() string {
	breakdown := ""
	stages := make([]string, 0, len(s.ByStage))
	for stage := range s.ByStage {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	for i, stage := range stages {
		if i > 0 {
			breakdown += " "
		}
		breakdown += stage + "=" + strconv.Itoa(s.ByStage[stage].TotalTokens)
	}
	if breakdown != "" {
		breakdown = " [" + breakdown + "]"
	}
	return "token usage: input=" + strconv.Itoa(s.Totals.InputTokens) +
		" output=" + strconv.Itoa(s.Totals.OutputTokens) +
		" total=" + strconv.Itoa(s.Totals.TotalTokens) +
		" calls=" + strconv.Itoa(s.Totals.Calls) + breakdown
}

func addCounts(a, b UsageCounts) UsageCounts {
	return UsageCounts{
		Calls:        a.Calls + b.Calls,
		InputTokens:  a.InputTokens + b.InputTokens,
		OutputTokens: a.OutputTokens + b.OutputTokens,
		TotalTokens:  a.TotalTokens + b.TotalTokens,
	}
}

// CountingLLMProvider wraps an LLMProvider and records every successful
// response's token usage into a UsageTracker.
type CountingLLMProvider struct {
	inner   LLMProvider
	tracker *UsageTracker
}

// WrapWithUsageTracker wraps inner so all its responses feed the tracker.
func WrapWithUsageTracker(inner LLMProvider, tracker *UsageTracker) *CountingLLMProvider {
	return &CountingLLMProvider{inner: inner, tracker: tracker}
}

func (p *CountingLLMProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	resp, err := p.inner.Chat(ctx, req)
	if err == nil {
		p.tracker.Record(resp, req.Metadata)
	}
	return resp, err
}
