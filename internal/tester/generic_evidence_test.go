package tester

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/store"
)

func TestGenericEvidenceTesterRequiresStateEvidence(t *testing.T) {
	ctx := context.Background()
	sqlite, project, task, attempt := genericEvidenceFixture(t)
	defer sqlite.Close()

	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "agent.finish", Summary: "done"}); err != nil {
		t.Fatalf("finish observation: %v", err)
	}
	tester := NewGenericEvidenceTester(sqlite)
	if _, err := tester.Test(ctx, task, attempt); err == nil {
		t.Fatal("expected missing state evidence to fail")
	}
	_ = project
}

func TestGenericEvidenceTesterPassesWithToolStateAndFinish(t *testing.T) {
	ctx := context.Background()
	sqlite, _, task, attempt := genericEvidenceFixture(t)
	defer sqlite.Close()

	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "state.file_changed", Summary: "wrote file"}); err != nil {
		t.Fatalf("state observation: %v", err)
	}
	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "agent.finish", Summary: "done"}); err != nil {
		t.Fatalf("finish observation: %v", err)
	}
	if _, err := sqlite.CreateToolCall(ctx, domain.ToolCall{AttemptID: attempt.ID, Name: "file.write", Status: domain.ToolCallStatusSucceeded}); err != nil {
		t.Fatalf("tool call: %v", err)
	}
	tester := NewGenericEvidenceTester(sqlite)
	result, err := tester.Test(ctx, task, attempt)
	if err != nil {
		t.Fatalf("test evidence: %v", err)
	}
	if result.TestResult.Status != domain.TestStatusPassed {
		t.Fatalf("expected passed, got %s", result.TestResult.Status)
	}
}

func TestGenericEvidenceTesterRequiresTestRunWhenAcceptanceMentionsTests(t *testing.T) {
	ctx := context.Background()
	sqlite, _, task, attempt := genericEvidenceFixture(t)
	defer sqlite.Close()
	task.AcceptanceCriteria = []string{"tests pass"}

	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "state.file_changed", Summary: "wrote file"}); err != nil {
		t.Fatalf("state observation: %v", err)
	}
	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "agent.finish", Summary: "done"}); err != nil {
		t.Fatalf("finish observation: %v", err)
	}
	tester := NewGenericEvidenceTester(sqlite)
	if _, err := tester.Test(ctx, task, attempt); err == nil {
		t.Fatal("expected tests acceptance without test.run to fail")
	}
}

func genericEvidenceFixture(t *testing.T) (*store.SQLiteStore, domain.Project, domain.Task, domain.TaskAttempt) {
	t.Helper()
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "evidence", Goal: "verify evidence"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Evidence task",
		Description:        "needs evidence",
		AcceptanceCriteria: []string{"evidence exists"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}
	inProgress, err := sqlite.TransitionTask(ctx, ready.ID, domain.TaskStatusInProgress, "start")
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, inProgress.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	implemented, err := sqlite.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "implemented")
	if err != nil {
		t.Fatalf("implemented task: %v", err)
	}
	return sqlite, project, implemented, attempt
}
