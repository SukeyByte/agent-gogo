package runtime

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/communication"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/executor"
	"github.com/sukeke/agent-gogo/internal/planner"
	"github.com/sukeke/agent-gogo/internal/reviewer"
	"github.com/sukeke/agent-gogo/internal/scheduler"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tester"
	"github.com/sukeke/agent-gogo/internal/validator"
)

func TestServiceRunsMinimalRuntimeLoop(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	service := NewService(sqlite)
	project, err := service.CreateProject(ctx, CreateProjectRequest{
		Name: "M3",
		Goal: "Run the minimal runtime loop",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.ID == "" {
		t.Fatal("expected project id")
	}

	planned, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("plan project: %v", err)
	}
	if len(planned) != 1 {
		t.Fatalf("expected one fixed task, got %d", len(planned))
	}
	if planned[0].Status != domain.TaskStatusReady {
		t.Fatalf("expected planned task to be READY, got %s", planned[0].Status)
	}

	result, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		t.Fatalf("run next task: %v", err)
	}
	if result.Task.Status != domain.TaskStatusDone {
		t.Fatalf("expected task to be DONE, got %s", result.Task.Status)
	}
	if result.Attempt.Number != 1 {
		t.Fatalf("expected attempt number 1, got %d", result.Attempt.Number)
	}
	if result.Attempt.Status != domain.AttemptStatusSucceeded {
		t.Fatalf("expected attempt to be SUCCEEDED, got %s", result.Attempt.Status)
	}
	if result.TestResult.Status != domain.TestStatusPassed {
		t.Fatalf("expected passing test result, got %s", result.TestResult.Status)
	}
	if result.ReviewResult.Status != domain.ReviewStatusApproved {
		t.Fatalf("expected approved review result, got %s", result.ReviewResult.Status)
	}

	gotEvents := map[string]bool{}
	for _, event := range result.Events {
		gotEvents[event.Type] = true
	}
	for _, eventType := range []string{
		"task.status_changed",
		"task_attempt.created",
		"task_attempt.completed",
	} {
		if !gotEvents[eventType] {
			t.Fatalf("expected event %q in %#v", eventType, result.Events)
		}
	}
	if len(result.Events) < 7 {
		t.Fatalf("expected lifecycle events to be recorded, got %d", len(result.Events))
	}
}

func TestServiceEmitsCommunicationIntents(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	outbox := communication.NewMemoryOutbox()
	commRuntime := communication.NewRuntime(outbox, communication.NewRenderer())
	web := communication.NewWebConsoleAdapter("web")
	commRuntime.RegisterChannel("web", web)

	service := NewService(sqlite)
	service.UseCommunication("web", "session-1", commRuntime)

	project, err := service.CreateProject(ctx, CreateProjectRequest{
		Name: "M5",
		Goal: "Emit communication intents",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := service.PlanProject(ctx, project.ID); err != nil {
		t.Fatalf("plan project: %v", err)
	}
	if _, err := service.RunNextTask(ctx, project.ID); err != nil {
		t.Fatalf("run next task: %v", err)
	}

	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 communication records, got %d", len(records))
	}
	if records[2].Intent.Type != communication.IntentNotifyDone {
		t.Fatalf("expected notify_done intent, got %s", records[2].Intent.Type)
	}
	messages := web.Messages()
	if len(messages) != 3 {
		t.Fatalf("expected 3 web messages, got %d", len(messages))
	}
	if messages[2].Type != communication.IntentNotifyDone {
		t.Fatalf("expected web notify_done message, got %s", messages[2].Type)
	}
}

func TestServicePersistsPlannerDependenciesAndSchedulerHonorsDAG(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	service := NewServiceWithComponents(
		sqlite,
		dependencyPlanner{},
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		executor.NewMinimalExecutor(sqlite),
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	project, err := service.CreateProject(ctx, CreateProjectRequest{
		Name: "DAG",
		Goal: "Run tasks in dependency order",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := service.PlanProject(ctx, project.ID); err != nil {
		t.Fatalf("plan project: %v", err)
	}

	first, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		t.Fatalf("run first task: %v", err)
	}
	if first.Task.Title != "Outline mystery" {
		t.Fatalf("expected dependency task first, got %q", first.Task.Title)
	}
	second, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		t.Fatalf("run second task: %v", err)
	}
	if second.Task.Title != "Write mystery" {
		t.Fatalf("expected dependent task second, got %q", second.Task.Title)
	}
}

type dependencyPlanner struct{}

func (p dependencyPlanner) PlanProject(ctx context.Context, req planner.PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Task{
		{
			ProjectID:          req.Project.ID,
			Title:              "Outline mystery",
			Description:        "Create the clue map",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"outline exists"},
		},
		{
			ProjectID:          req.Project.ID,
			Title:              "Write mystery",
			Description:        "Write the short story",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"story exists"},
			DependsOn:          []string{"Outline mystery"},
		},
	}, nil
}
