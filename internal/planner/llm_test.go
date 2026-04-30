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
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected planner to request structured json, got %#v", req.ResponseFormat)
		}
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
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks with research/reflection prefix, got %d", len(tasks))
	}
	if tasks[0].Title != "研究上下文与可用资料" {
		t.Fatalf("expected research task first, got %q", tasks[0].Title)
	}
	if tasks[1].Title != "反思任务拆解与验收口径" {
		t.Fatalf("expected reflection task second, got %q", tasks[1].Title)
	}
	if tasks[0].Status != domain.TaskStatusDraft {
		t.Fatalf("expected draft task, got %s", tasks[0].Status)
	}
	if len(tasks[0].AcceptanceCriteria) == 0 {
		t.Fatal("expected acceptance criteria")
	}
}

func TestEnsureResearchAndReflectionPreservesResearchBeforeReflection(t *testing.T) {
	out := ensureResearchAndReflectionTasks(PlanRequest{
		Project: domain.Project{Goal: "根据 README 写项目简介文档"},
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
		IntentProfile: intent.Profile{
			TaskType:   "document",
			Complexity: "high",
		},
	}, []plannedTask{
		{
			Title:      "读取 README 文件",
			Goal:       "读取项目 README 内容",
			Type:       "runtime",
			Acceptance: []string{"成功读取 README 文件内容"},
		},
		{
			Title:      "撰写项目简介文档",
			Goal:       "根据 README 内容写简介",
			Type:       "document",
			Acceptance: []string{"文档包含项目名称"},
		},
	})
	if len(out) != 3 {
		t.Fatalf("expected research, reflection, implementation tasks, got %#v", out)
	}
	if out[0].Title != "读取 README 文件" {
		t.Fatalf("expected existing research task first, got %q", out[0].Title)
	}
	if out[1].Title != "反思任务拆解与验收口径" {
		t.Fatalf("expected injected reflection second, got %q", out[1].Title)
	}
	if len(out[1].DependsOn) != 1 || out[1].DependsOn[0] != "读取 README 文件" {
		t.Fatalf("expected reflection to depend on research, got %#v", out[1].DependsOn)
	}
	if len(out[2].DependsOn) != 1 || out[2].DependsOn[0] != "反思任务拆解与验收口径" {
		t.Fatalf("expected implementation to depend on reflection, got %#v", out[2].DependsOn)
	}
}
