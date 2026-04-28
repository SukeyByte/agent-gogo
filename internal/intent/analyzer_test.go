package intent

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/provider"
)

func TestLLMAnalyzerUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"task_type":"code",
				"complexity":"L3",
				"domains":["go","runtime","go"],
				"required_capabilities":["test.run","code.search"],
				"risk_level":"medium",
				"needs_user_confirmation":false,
				"grounding_requirement":"tests",
				"confidence":0.92
			}`,
		}, nil
	})
	analyzer := NewLLMAnalyzer(llm, "gpt-test")
	profile, err := analyzer.Analyze(context.Background(), Request{
		UserInput: "fix runtime",
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if profile.TaskType != "code" {
		t.Fatalf("expected code task, got %s", profile.TaskType)
	}
	if len(profile.Domains) != 2 {
		t.Fatalf("expected deduped domains, got %#v", profile.Domains)
	}
	if profile.ContextProfile().GroundingRequirement != "tests" {
		t.Fatalf("expected context profile grounding")
	}
}
