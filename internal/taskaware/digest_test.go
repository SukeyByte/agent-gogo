package taskaware

import (
	"context"
	"strings"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/store"
)

func TestBuildContextSnapshotCarriesProjectAndTaskAwareness(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "W9", Goal: "Make agent task-aware"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	first, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Discover current state",
		Description:        "Inspect project state and write an observation.",
		AcceptanceCriteria: []string{"state is known"},
	})
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	second, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Use prior knowledge",
		Description:        "Use digest and memory from the first task.",
		AcceptanceCriteria: []string{"prior memory is visible"},
	})
	if err != nil {
		t.Fatalf("create second task: %v", err)
	}
	if _, err := sqlite.CreateTaskDependency(ctx, domain.TaskDependency{TaskID: second.ID, DependsOnTaskID: first.ID}); err != nil {
		t.Fatalf("create dependency: %v", err)
	}

	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusReady)
	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusInProgress)
	attempt, err := sqlite.CreateTaskAttempt(ctx, first.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if _, err := sqlite.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "executor.summary",
		Summary:     "Project has one follow-up task that depends on this discovery.",
		EvidenceRef: "evidence://discovery",
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	if _, err := sqlite.CreateTestResult(ctx, domain.TestResult{
		AttemptID:   attempt.ID,
		Name:        "digest-test",
		Status:      domain.TestStatusPassed,
		Output:      "digest source verified",
		EvidenceRef: "exec://test.run",
	}); err != nil {
		t.Fatalf("create test result: %v", err)
	}
	review, err := sqlite.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID:   attempt.ID,
		Status:      domain.ReviewStatusApproved,
		Summary:     "Discovery is reliable and should guide the next task.",
		EvidenceRef: "review://approved",
	})
	if err != nil {
		t.Fatalf("create review: %v", err)
	}
	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusImplemented)
	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusTesting)
	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusReviewing)
	first = mustTransition(t, sqlite, ctx, first.ID, domain.TaskStatusDone)
	if _, err := sqlite.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusSucceeded, "approved"); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}
	second = mustTransition(t, sqlite, ctx, second.ID, domain.TaskStatusReady)

	snapshot, err := BuildContextSnapshot(ctx, sqlite, project, second.ID)
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snapshot.ProjectState.Digest.TaskCount != 2 {
		t.Fatalf("expected two tasks, got %#v", snapshot.ProjectState.Digest)
	}
	if len(snapshot.ProjectState.Digest.CompletedTasks) != 1 || snapshot.ProjectState.Digest.CompletedTasks[0].Title != "Discover current state" {
		t.Fatalf("expected completed first task, got %#v", snapshot.ProjectState.Digest.CompletedTasks)
	}
	if len(snapshot.TaskState.DependsOn) != 1 || snapshot.TaskState.DependsOn[0].ID != first.ID {
		t.Fatalf("expected current task dependency, got %#v", snapshot.TaskState.DependsOn)
	}
	if len(snapshot.ProjectState.Digest.Decisions) != 1 || !strings.Contains(snapshot.ProjectState.Digest.Decisions[0].Summary, "Discovery is reliable") {
		t.Fatalf("expected review decision, got %#v", snapshot.ProjectState.Digest.Decisions)
	}
	if !strings.Contains(snapshot.QueryText, "follow-up task") {
		t.Fatalf("expected observation in memory query, got %q", snapshot.QueryText)
	}

	memories, err := ExtractTaskMemories(ctx, sqlite, project, first, attempt, review)
	if err != nil {
		t.Fatalf("extract memories: %v", err)
	}
	if len(memories) == 0 {
		t.Fatal("expected extracted memories")
	}
	if memories[0].SourceTaskID != first.ID || memories[0].SourceAttemptID != attempt.ID {
		t.Fatalf("expected source trace, got %#v", memories[0].Card)
	}
}

func mustTransition(t *testing.T, sqlite *store.SQLiteStore, ctx context.Context, taskID string, status domain.TaskStatus) domain.Task {
	t.Helper()
	task, err := sqlite.TransitionTask(ctx, taskID, status, "test transition")
	if err != nil {
		t.Fatalf("transition %s: %v", status, err)
	}
	return task
}
