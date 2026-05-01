package chain

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/provider"
)

func TestLLMRouterUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected structured response format, got %#v", req.ResponseFormat)
		}
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
				"requires_dag":true,
				"estimated_steps":5,
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
				"requires_dag":false,
				"estimated_steps":2,
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

func TestLLMRouterPromotesProjectScaleFromAISignals(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"level":"L2",
				"reason":"AI estimates a multi-phase runtime refactor with many leaf tasks",
				"need_plan":true,
				"need_tools":true,
				"need_memory":true,
				"need_review":true,
				"need_browser":false,
				"need_code":true,
				"need_docs":true,
				"requires_dag":true,
				"estimated_steps":7,
				"persona_ids":["planner"],
				"skill_tags":["go","runtime"],
				"tool_names":["code.search","file.read","test.run"],
				"risk_level":"medium"
			}`,
		}, nil
	})
	router := NewLLMRouter(llm, "gpt-test")
	decision, err := router.Route(context.Background(), Request{
		UserInput: "把 runtime、web console、capability resolver、observer 和 memory 收敛成通用 agent 主链路",
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if decision.Level != LevelProject {
		t.Fatalf("expected AI scale signals to promote to L3, got %#v", decision)
	}
	if !decision.RequiresDAG || decision.EstimatedSteps != 7 {
		t.Fatalf("expected structured scale signals to be preserved, got %#v", decision)
	}
}

func TestLLMRouterDoesNotPromoteDirectBrowserActionSequence(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"level":"L2",
				"reason":"browser action sequence with several tool calls but no project DAG",
				"need_plan":true,
				"need_tools":true,
				"need_memory":false,
				"need_review":false,
				"need_browser":true,
				"need_code":false,
				"need_docs":false,
				"requires_dag":false,
				"estimated_steps":5,
				"persona_ids":["browser-agent"],
				"skill_tags":["browser-automation"],
				"tool_names":["browser.open","browser.input","browser.click","browser.wait"],
				"risk_level":"low"
			}`,
		}, nil
	})
	router := NewLLMRouter(llm, "gpt-test")
	decision, err := router.Route(context.Background(), Request{
		UserInput: "open a local page, type text, click a button, and wait for result",
	})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if decision.Level == LevelProject || IsProjectScale(decision) {
		t.Fatalf("expected direct browser action sequence to stay planned, got %#v", decision)
	}
}
