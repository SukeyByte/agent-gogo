package runtime

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/validator"
)

// scriptedProjectReviewer rejects the first N rounds with given gaps.
type scriptedProjectReviewer struct {
	rejections int // remaining rejections
	gaps       []string
	calls      int
}

func (r *scriptedProjectReviewer) ReviewProject(ctx context.Context, input reviewer.ProjectReviewInput) (reviewer.ProjectReview, error) {
	r.calls++
	if r.rejections > 0 {
		r.rejections--
		return reviewer.ProjectReview{Approved: false, Summary: "integration gaps found", Gaps: r.gaps}, nil
	}
	return reviewer.ProjectReview{Approved: true, Summary: "goal achieved end to end"}, nil
}

// finishingExecutor completes every task with finish evidence.
type finishingExecutor struct{ store *store.SQLiteStore }

func (e *finishingExecutor) Execute(ctx context.Context, task domain.Task) (executor.Result, error) {
	inProgress := task
	if task.Status != domain.TaskStatusInProgress {
		claimed, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "finishing executor started")
		if err != nil {
			return executor.Result{}, err
		}
		inProgress = claimed
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return executor.Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID: attempt.ID, Type: "agent.finish", Summary: "finished " + task.Title,
	}); err != nil {
		return executor.Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "done")
	if err != nil {
		return executor.Result{}, err
	}
	return executor.Result{Task: implemented, Attempt: attempt}, nil
}

func seedReadyTask(t *testing.T, service *Service, title string) domain.Task {
	t.Helper()
	ctx := context.Background()
	project, err := service.store.CreateProject(ctx, domain.Project{Name: title, Goal: "final review test"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := service.store.CreateTask(ctx, domain.Task{
		ProjectID: project.ID, Title: title, AcceptanceCriteria: []string{"done"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := service.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "seed")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}
	return ready
}

func TestFinalReviewRejectThenDeltaRoundApproves(t *testing.T) {
	ctx := context.Background()
	pr := &scriptedProjectReviewer{rejections: 1, gaps: []string{"deliverables are not connected end to end"}}
	service := rebuildWithStore(t, nil, pr)
	task := seedReadyTask(t, service, "Build widget")

	ran, err := service.RunProjectTasks(ctx, task.ProjectID, 10)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	project, err := service.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if project.Status != domain.ProjectStatusCompleted {
		t.Fatalf("project status = %s, want COMPLETED", project.Status)
	}
	if pr.calls != 2 {
		t.Fatalf("expected 2 final reviews (reject then approve), got %d", pr.calls)
	}
	tasks, _ := service.store.ListTasksByProject(ctx, task.ProjectID)
	deltaCount := 0
	for _, tk := range tasks {
		if len(tk.Title) > 5 && tk.Title[:5] == "Delta" {
			deltaCount++
			if tk.Status != domain.TaskStatusDone {
				t.Fatalf("delta task %q status = %s", tk.Title, tk.Status)
			}
		}
	}
	if deltaCount != 1 {
		t.Fatalf("expected 1 delta task executed, got %d", deltaCount)
	}
	if ran < 2 {
		t.Fatalf("expected at least 2 runs (original + delta), got %d", ran)
	}
}

func TestFinalReviewBudgetExhaustionBlocks(t *testing.T) {
	ctx := context.Background()
	pr := &scriptedProjectReviewer{rejections: 10, gaps: []string{"goal not met"}}
	service := rebuildWithStore(t, nil, pr)
	task := seedReadyTask(t, service, "Build unfinishable")

	if _, err := service.RunProjectTasks(ctx, task.ProjectID, 30); err != nil {
		t.Fatalf("run should end blocked, not error: %v", err)
	}
	project, _ := service.store.GetProject(ctx, task.ProjectID)
	if project.Status == domain.ProjectStatusCompleted {
		t.Fatal("project must not be completed when final review keeps rejecting")
	}
	if got := service.projectReplanCount(ctx, task.ProjectID); got != maxProjectReplans {
		t.Fatalf("replan rounds = %d, want %d", got, maxProjectReplans)
	}
	tasks, _ := service.store.ListTasksByProject(ctx, task.ProjectID)
	for _, tk := range tasks {
		if tk.Status == domain.TaskStatusReady {
			t.Fatalf("no READY tasks should remain after budget exhaustion, found %q", tk.Title)
		}
	}
}

// rebuildWithStore builds the service with a shared store so the executor
// and seeder see the same data.
func rebuildWithStore(t *testing.T, _ *Service, pr ProjectReviewer) *Service {
	t.Helper()
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	service := NewServiceWithComponents(
		sqlite,
		planner.NewFixedPlanner(),
		validator.NewMinimalTaskValidator(),
		scheduler.NewClaimingScheduler(sqlite, sqlite),
		&finishingExecutor{store: sqlite},
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseProjectReviewer(pr)
	return service
}
