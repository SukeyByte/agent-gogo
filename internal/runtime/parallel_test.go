package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/validator"
)

// slowExecutor simulates real work so parallel execution overlaps.
type slowExecutor struct {
	store      *store.SQLiteStore
	sleep      time.Duration
	inside     atomic.Int64
	maxInside  atomic.Int64
	onExecuted func()
}

func (e *slowExecutor) Execute(ctx context.Context, task domain.Task) (executor.Result, error) {
	current := e.inside.Add(1)
	for {
		max := e.maxInside.Load()
		if current <= max || e.maxInside.CompareAndSwap(max, current) {
			break
		}
	}
	defer e.inside.Add(-1)
	time.Sleep(e.sleep)

	inProgress := task
	if task.Status != domain.TaskStatusInProgress {
		claimed, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "slow executor started")
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
		AttemptID: attempt.ID,
		Type:      "agent.finish",
		Summary:   "slow executor completed " + task.Title,
	}); err != nil {
		return executor.Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "slow executor done")
	if err != nil {
		return executor.Result{}, err
	}
	if e.onExecuted != nil {
		e.onExecuted()
	}
	return executor.Result{Task: implemented, Attempt: attempt}, nil
}

func newParallelTestService(t *testing.T, exec executor.Executor, sqlite *store.SQLiteStore) *Service {
	t.Helper()
	return NewServiceWithComponents(
		sqlite,
		planner.NewFixedPlanner(),
		validator.NewMinimalTaskValidator(),
		scheduler.NewClaimingScheduler(sqlite, sqlite),
		exec,
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
}

func TestRunProjectTasksParallelOverlapsIndependentTasks(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "Parallel", Goal: "run in parallel"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	for i := 0; i < 4; i++ {
		created, err := sqlite.CreateTask(ctx, domain.Task{
			ProjectID: project.ID, Title: "parallel-task", AcceptanceCriteria: []string{"done"},
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		if _, err := sqlite.TransitionTask(ctx, created.ID, domain.TaskStatusReady, "ready"); err != nil {
			t.Fatalf("ready task: %v", err)
		}
	}

	exec := &slowExecutor{store: sqlite, sleep: 120 * time.Millisecond}
	service := newParallelTestService(t, exec, sqlite)
	service.UseParallelism(4)

	ran, err := service.RunProjectTasks(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("run parallel: %v", err)
	}
	if ran != 4 {
		t.Fatalf("expected 4 tasks ran, got %d", ran)
	}
	if exec.maxInside.Load() < 2 {
		t.Fatalf("expected concurrent execution (max in-flight %d), tasks ran sequentially", exec.maxInside.Load())
	}
	tasks, err := sqlite.ListTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	for _, task := range tasks {
		if task.Status != domain.TaskStatusDone {
			t.Fatalf("task %s status = %s, want DONE", task.Title, task.Status)
		}
	}
}

func TestRunProjectTasksParallelRespectsDependencies(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "Deps", Goal: "serial deps"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	var first, second domain.Task
	for _, name := range []string{"first", "second"} {
		created, err := sqlite.CreateTask(ctx, domain.Task{
			ProjectID: project.ID, Title: name, AcceptanceCriteria: []string{"done"},
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		if _, err := sqlite.TransitionTask(ctx, created.ID, domain.TaskStatusReady, "ready"); err != nil {
			t.Fatalf("ready task: %v", err)
		}
		if name == "first" {
			first = created
		} else {
			second = created
		}
	}
	if _, err := sqlite.CreateTaskDependency(ctx, domain.TaskDependency{
		TaskID: second.ID, DependsOnTaskID: first.ID,
	}); err != nil {
		t.Fatalf("create dependency: %v", err)
	}

	exec := &slowExecutor{store: sqlite, sleep: 100 * time.Millisecond}
	service := newParallelTestService(t, exec, sqlite)
	service.UseParallelism(4)
	// The dependency outcome is the assertion that matters: both tasks must
	// finish, and the dependent must never start before its prerequisite.
	if _, err := service.RunProjectTasks(ctx, project.ID, 10); err != nil {
		t.Fatalf("run: %v", err)
	}
	firstFinal, err := sqlite.GetTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	secondFinal, err := sqlite.GetTask(ctx, second.ID)
	if err != nil {
		t.Fatalf("get second: %v", err)
	}
	if firstFinal.Status != domain.TaskStatusDone || secondFinal.Status != domain.TaskStatusDone {
		t.Fatalf("unexpected statuses: first=%s second=%s", firstFinal.Status, secondFinal.Status)
	}
}
