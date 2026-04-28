package tools

import (
	"context"
	"os"
	"path/filepath"
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
		Title:              "Call registered tool",
		AcceptanceCriteria: []string{"tool call is audited"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	runtime := NewRuntime(sqlite)
	runtime.Register(Spec{Name: "test.run", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		return Result{
			Success: true,
			Output: map[string]any{
				"command": command,
				"passed":  true,
				"summary": "tests passed",
			},
			EvidenceRef: "exec://tool/test.run",
		}, nil
	})
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
	if response.ToolCall.EvidenceRef != "exec://tool/test.run" {
		t.Fatalf("expected evidence ref, got %q", response.ToolCall.EvidenceRef)
	}
	if !strings.Contains(response.ToolCall.InputJSON, "go test ./...") {
		t.Fatalf("expected input json to include command, got %s", response.ToolCall.InputJSON)
	}
	if !strings.Contains(response.ToolCall.OutputJSON, "tests passed") {
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

	runtime := NewRuntime(sqlite)
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

func TestBuiltinRuntimeCodeSearch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "sample.go"), []byte("package internal\n\nfunc Needle() {}\n"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	runtime := NewBuiltinRuntime(nil, root)
	response, err := runtime.Call(ctx, CallRequest{
		Name: "code.search",
		Args: map[string]any{
			"query": "Needle",
			"paths": []any{"internal"},
		},
	})
	if err != nil {
		t.Fatalf("search code: %v", err)
	}
	matches, ok := response.Result.Output["matches"].([]map[string]any)
	if !ok || len(matches) != 1 {
		t.Fatalf("expected one match, got %#v", response.Result.Output["matches"])
	}
	if matches[0]["path"] != "internal/sample.go" {
		t.Fatalf("expected relative path, got %#v", matches[0]["path"])
	}
}
