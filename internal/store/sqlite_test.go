package store

import (
	"context"
	"strings"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
)

func TestSQLiteStoreCreatesProjectTaskAttemptAndEvents(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, domain.Project{
		Name: "M1",
		Goal: "Build domain and store",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.ID == "" {
		t.Fatal("expected project id")
	}

	task, err := store.CreateTask(ctx, domain.Task{
		ProjectID:            project.ID,
		Title:                "Create schema",
		Description:          "Build M1 database tables",
		Phase:                "Implementation",
		AcceptanceCriteria:   []string{"schema exists", "tests pass"},
		RequiredCapabilities: []string{"write", "verify"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != domain.TaskStatusDraft {
		t.Fatalf("expected DRAFT status, got %s", task.Status)
	}
	loadedTask, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loadedTask.Phase != "Implementation" || strings.Join(loadedTask.RequiredCapabilities, ",") != "write,verify" {
		t.Fatalf("expected task planning metadata to round-trip, got phase=%q caps=%v", loadedTask.Phase, loadedTask.RequiredCapabilities)
	}

	ready, err := store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "validated")
	if err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if ready.Status != domain.TaskStatusReady {
		t.Fatalf("expected READY status, got %s", ready.Status)
	}

	attempt, err := store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if attempt.Number != 1 {
		t.Fatalf("expected attempt number 1, got %d", attempt.Number)
	}

	_, err = store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID: attempt.ID,
		Name:      "sample.tool",
		InputJSON: `{"ok":true}`,
		Status:    domain.ToolCallStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	_, err = store.CreateObservation(ctx, domain.Observation{
		AttemptID: attempt.ID,
		Type:      "tool.output",
		Summary:   "sample tool completed",
	})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	_, err = store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "smoke",
		Status:    domain.TestStatusPassed,
	})
	if err != nil {
		t.Fatalf("create test result: %v", err)
	}
	_, err = store.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID: attempt.ID,
		Status:    domain.ReviewStatusApproved,
		Summary:   "accepted",
	})
	if err != nil {
		t.Fatalf("create review result: %v", err)
	}
	_, err = store.CreateArtifact(ctx, domain.Artifact{
		AttemptID: attempt.ID,
		ProjectID: project.ID,
		Type:      "text",
		Path:      "artifacts/example.txt",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "task.status_changed" {
		t.Fatalf("expected first event to be status change, got %s", events[0].Type)
	}
	if events[1].Type != "task_attempt.created" {
		t.Fatalf("expected second event to be attempt creation, got %s", events[1].Type)
	}
}

func TestSQLiteStoreRejectsInvalidTaskTransition(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, domain.Project{Name: "M1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := store.CreateTask(ctx, domain.Task{
		ProjectID: project.ID,
		Title:     "Invalid jump",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, "skip lifecycle"); err == nil {
		t.Fatal("expected invalid transition error")
	}

	loaded, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loaded.Status != domain.TaskStatusDraft {
		t.Fatalf("expected task to remain DRAFT, got %s", loaded.Status)
	}
}

func TestTaskEventsAreAppendOnlyInSQLite(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, domain.Project{Name: "M1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := store.CreateTask(ctx, domain.Task{
		ProjectID: project.ID,
		Title:     "Append-only event",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	event, err := store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:  task.ID,
		Type:    "test.event",
		Message: "cannot mutate",
	})
	if err != nil {
		t.Fatalf("add event: %v", err)
	}

	_, err = store.DB().ExecContext(ctx, "UPDATE task_events SET message = 'changed' WHERE id = ?", event.ID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only update error, got %v", err)
	}

	_, err = store.DB().ExecContext(ctx, "DELETE FROM task_events WHERE id = ?", event.ID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only delete error, got %v", err)
	}
}
