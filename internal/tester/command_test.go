package tester

import (
	"context"
	"errors"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tools"
)

func TestCommandTesterFailsFromRealToolResult(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "M8", Goal: "test feedback"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{ProjectID: project.ID, Title: "Run tests", Status: domain.TaskStatusImplemented, AcceptanceCriteria: []string{"tests fail"}})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	tester := NewCommandTester(sqlite, failingTool{}, "go test ./...")

	result, err := tester.Test(ctx, task, attempt)
	if err == nil {
		t.Fatal("expected tester failure")
	}
	if result.TestResult.Status != domain.TestStatusFailed {
		t.Fatalf("expected failed result, got %s", result.TestResult.Status)
	}
	if result.Task.Status != domain.TaskStatusFailed {
		t.Fatalf("expected failed task, got %s", result.Task.Status)
	}
}

type failingTool struct{}

func (failingTool) Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error) {
	return tools.CallResponse{
		Result: tools.Result{
			Success:     false,
			Error:       "exit status 1",
			Output:      map[string]any{"summary": "tests failed"},
			EvidenceRef: "exec://test.run",
		},
		ToolCall: domain.ToolCall{EvidenceRef: "exec://test.run"},
	}, errors.New("exit status 1")
}
