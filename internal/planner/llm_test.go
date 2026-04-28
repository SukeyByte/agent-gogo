package planner

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/intent"
	"github.com/sukeke/agent-gogo/internal/provider"
)

func TestLLMPlannerUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"tasks":[
					{
						"title":"Scan project",
						"goal":"Identify project structure and tests",
						"type":"code",
						"depends_on":[],
						"acceptance":["project structure summarized","test command identified"]
					},
					{
						"title":"Run tests",
						"goal":"Run current test suite",
						"type":"code",
						"depends_on":["Scan project"],
						"acceptance":["tests executed","result recorded"]
					}
				]
			}`,
		}, nil
	})
	planner := NewLLMPlanner(llm, "gpt-test")
	tasks, err := planner.PlanProject(context.Background(), PlanRequest{
		Project: domain.Project{ID: "project-1", Name: "agent-gogo", Goal: "make it real"},
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
		IntentProfile: intent.Profile{
			TaskType: "code",
		},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Status != domain.TaskStatusDraft {
		t.Fatalf("expected draft task, got %s", tasks[0].Status)
	}
	if len(tasks[0].AcceptanceCriteria) == 0 {
		t.Fatal("expected acceptance criteria")
	}
}
