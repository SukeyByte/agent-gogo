package discovery

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/intent"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

func TestToolLoopUsesBrowserForURLDiscovery(t *testing.T) {
	runtime := &recordingRuntime{}
	loop := NewToolLoop(runtime)
	result, err := loop.Discover(context.Background(), Request{
		Project:       domain.Project{Goal: "读 https://example.com 并总结主要内容"},
		ChainDecision: chain.Decision{NeedBrowser: true},
		IntentProfile: intent.Profile{TaskType: "browser"},
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !runtime.called("browser.open") {
		t.Fatalf("expected browser.open discovery probe, got %#v", runtime.names)
	}
	if !runtime.called("browser.extract") {
		t.Fatalf("expected browser.extract discovery probe, got %#v", runtime.names)
	}
	if result.Summary == "" {
		t.Fatal("expected discovery summary")
	}
}

type recordingRuntime struct {
	names []string
}

func (r *recordingRuntime) Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error) {
	r.names = append(r.names, req.Name)
	return tools.CallResponse{
		Result: tools.Result{
			Success:     true,
			EvidenceRef: "evidence://" + req.Name,
			Output: map[string]any{
				"url":         req.Args["url"],
				"dom_summary": "Example Domain",
				"path":        req.Args["path"],
			},
		},
	}, nil
}

func (r *recordingRuntime) called(name string) bool {
	for _, got := range r.names {
		if got == name {
			return true
		}
	}
	return false
}
