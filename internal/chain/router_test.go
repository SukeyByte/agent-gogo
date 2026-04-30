package chain

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/provider"
)

func TestLLMRouterUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"level":"L3",
				"reason":"requires code changes and tests",
				"need_plan":true,
				"need_tools":true,
				"need_memory":true,
				"need_review":true,
				"need_browser":false,
				"need_code":true,
				"need_docs":false,
				"persona_ids":["reviewer","planner","planner"],
				"skill_tags":["go","code"],
				"tool_names":["test.run","code.search"],
				"risk_level":"medium"
			}`,
		}, nil
	})
	router := NewLLMRouter(llm, "gpt-test")
	decision, err := router.Route(context.Background(), Request{UserInput: "fix the tests"})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if decision.Level != LevelProject {
		t.Fatalf("expected L3, got %s", decision.Level)
	}
	if len(decision.PersonaIDs) != 2 {
		t.Fatalf("expected deduped personas, got %#v", decision.PersonaIDs)
	}
	if !decision.NeedCode || !decision.NeedReview {
		t.Fatalf("expected code and review flags, got %#v", decision)
	}
}

func TestLLMRouterAcceptsNumericRiskLevel(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"level":"L2",
				"reason":"needs tools",
				"need_plan":true,
				"need_tools":true,
				"need_memory":false,
				"need_review":true,
				"need_browser":false,
				"need_code":true,
				"need_docs":false,
				"persona_ids":[],
				"skill_tags":[],
				"tool_names":[],
				"risk_level":2
			}`,
		}, nil
	})
	router := NewLLMRouter(llm, "gpt-test")
	decision, err := router.Route(context.Background(), Request{UserInput: "fix the tests"})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if decision.RiskLevel != "medium" {
		t.Fatalf("expected medium risk, got %q", decision.RiskLevel)
	}
}
