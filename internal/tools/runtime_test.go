package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/store"
)

func TestRuntimeCallAuditsToolCall(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "M4", Goal: "tool audit"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Call mock tool",
		AcceptanceCriteria: []string{"tool call is audited"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	runtime := NewMockRuntime(sqlite)
	response, err := runtime.Call(ctx, CallRequest{
		AttemptID: attempt.ID,
		Name:      "test.run",
		Args: map[string]any{
			"command": "go test ./...",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !response.Result.Success {
		t.Fatal("expected successful result")
	}
	if response.ToolCall.Status != domain.ToolCallStatusSucceeded {
		t.Fatalf("expected succeeded tool call, got %s", response.ToolCall.Status)
	}
	if response.ToolCall.EvidenceRef != "mock://tool/test.run" {
		t.Fatalf("expected evidence ref, got %q", response.ToolCall.EvidenceRef)
	}
	if !strings.Contains(response.ToolCall.InputJSON, "go test ./...") {
		t.Fatalf("expected input json to include command, got %s", response.ToolCall.InputJSON)
	}
	if !strings.Contains(response.ToolCall.OutputJSON, "mock tests passed") {
		t.Fatalf("expected output json summary, got %s", response.ToolCall.OutputJSON)
	}
}

func TestRuntimeAuditsMissingToolFailure(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "M4", Goal: "tool audit"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Call missing tool",
		AcceptanceCriteria: []string{"failure is audited"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	runtime := NewMockRuntime(sqlite)
	response, err := runtime.Call(ctx, CallRequest{
		AttemptID: attempt.ID,
		Name:      "missing.tool",
		Args:      map[string]any{"ok": false},
	})
	if err == nil {
		t.Fatal("expected missing tool error")
	}
	if response.ToolCall.Status != domain.ToolCallStatusFailed {
		t.Fatalf("expected failed tool call, got %s", response.ToolCall.Status)
	}
	if response.ToolCall.Error != ErrToolNotFound.Error() {
		t.Fatalf("expected not found error, got %q", response.ToolCall.Error)
	}
}
