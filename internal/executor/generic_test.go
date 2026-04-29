package executor

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tools"
)

func TestGenericExecutorRunsToolActionLoop(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "write a file"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Write artifact",
		Description:        "Write a file through tool runtime",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{
			Success:     true,
			Output:      map[string]any{"path": args["path"]},
			EvidenceRef: "file://" + args["path"].(string),
		}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step == 1 {
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"write requested file"}`}, nil
		}
		return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written","reason":"tool evidence exists"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
	observations, err := sqlite.ListObservationsByAttempt(ctx, result.Attempt.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	types := map[string]bool{}
	for _, observation := range observations {
		types[observation.Type] = true
	}
	for _, typ := range []string{"state.file_changed", "agent.finish"} {
		if !types[typ] {
			t.Fatalf("expected observation %s in %#v", typ, observations)
		}
	}
}
