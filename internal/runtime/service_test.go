package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/communication"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/validator"
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
	if len(records) != 7 {
		t.Fatalf("expected 7 communication records, got %d", len(records))
	}
	if records[len(records)-1].Intent.Type != communication.IntentNotifyDone {
		t.Fatalf("expected notify_done intent, got %s", records[len(records)-1].Intent.Type)
	}
	messages := web.Messages()
	if len(messages) != 7 {
		t.Fatalf("expected 7 web messages, got %d", len(messages))
	}
	if messages[len(messages)-1].Type != communication.IntentNotifyDone {
		t.Fatalf("expected web notify_done message, got %s", messages[len(messages)-1].Type)
	}
	seenProgress := false
	for _, record := range records {
		if record.Intent.Type == communication.IntentSendProgress {
			seenProgress = true
		}
	}
	if !seenProgress {
		t.Fatal("expected progress notifications during task lifecycle")
	}
}

func TestHandleChannelEventAutoRunsProjectAndEmitsResult(t *testing.T) {
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

	service := NewServiceWithComponents(
		sqlite,
		dependencyPlanner{},
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		executor.NewMinimalExecutor(sqlite),
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseCommunication("web", "session-1", commRuntime)

	if err := service.HandleChannelEvent(ctx, ChannelEvent{
		Type: "goal.submitted",
		Text: "完成一个多阶段项目任务并汇报结果",
		Payload: map[string]string{
			"name": "Legendary goal",
		},
	}); err != nil {
		t.Fatalf("handle channel event: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var records []communication.OutboxRecord
	for time.Now().Before(deadline) {
		records, err = outbox.List(ctx)
		if err != nil {
			t.Fatalf("list outbox: %v", err)
		}
		for _, record := range records {
			if record.Intent.Type == communication.IntentNotifyDone && strings.Contains(record.Rendered.Text, "Project run finished") {
				projects, err := sqlite.ListProjects(ctx)
				if err != nil {
					t.Fatalf("list projects: %v", err)
				}
				tasks, err := sqlite.ListTasksByProject(ctx, projects[0].ID)
				if err != nil {
					t.Fatalf("list tasks: %v", err)
				}
				for _, task := range tasks {
					if task.Status != domain.TaskStatusDone {
						t.Fatalf("expected all channel-run tasks done, got %#v", tasks)
					}
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected project done notification, got %#v", records)
}

func TestChannelEventReportsPlanningCapabilityBlockWithoutHTTPFailure(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	outbox := communication.NewMemoryOutbox()
	commRuntime := communication.NewRuntime(outbox, communication.NewRenderer())
	commRuntime.RegisterChannel("web", communication.NewWebConsoleAdapter("web"))

	service := NewServiceWithComponents(
		sqlite,
		allBlockedPlanner{},
		titleBlockingValidator{},
		scheduler.NewReadyScheduler(sqlite),
		executor.NewMinimalExecutor(sqlite),
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseCommunication("web", "session-1", commRuntime)

	if err := service.HandleChannelEvent(ctx, ChannelEvent{
		Type:    "goal.submitted",
		Text:    "项目级任务需要不可用能力时也要通知 channel",
		Payload: map[string]string{"name": "Blocked project-scale goal"},
	}); err != nil {
		t.Fatalf("expected channel event to convert planning block into channel status, got %v", err)
	}

	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if !hasRenderedText(records, "Project blocked during planning") {
		t.Fatalf("expected project blocked notification, got %#v", records)
	}
}

func TestRunProjectTasksRunsReadyTasksAndReportsBlockedRemainder(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	outbox := communication.NewMemoryOutbox()
	commRuntime := communication.NewRuntime(outbox, communication.NewRenderer())
	commRuntime.RegisterChannel("web", communication.NewWebConsoleAdapter("web"))

	service := NewServiceWithComponents(
		sqlite,
		partialBlockedPlanner{},
		titleBlockingValidator{},
		scheduler.NewReadyScheduler(sqlite),
		executor.NewMinimalExecutor(sqlite),
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseCommunication("web", "session-1", commRuntime)
	project, err := service.CreateProject(ctx, CreateProjectRequest{Name: "Partial", Goal: "run what is safe and report blocked work"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	planned, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("plan project: %v", err)
	}
	if len(planned) != 1 || planned[0].Title != "Readable slice" {
		t.Fatalf("expected one runnable task, got %#v", planned)
	}
	ran, err := service.RunProjectTasks(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("run project tasks: %v", err)
	}
	if ran != 1 {
		t.Fatalf("expected one runnable task to run, got %d", ran)
	}
	tasks, err := sqlite.ListTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	statusByTitle := map[string]domain.TaskStatus{}
	for _, task := range tasks {
		statusByTitle[task.Title] = task.Status
	}
	if statusByTitle["Readable slice"] != domain.TaskStatusDone {
		t.Fatalf("expected readable task done, got %#v", statusByTitle)
	}
	if statusByTitle["Blocked implementation"] != domain.TaskStatusBlocked {
		t.Fatalf("expected blocked task to remain blocked, got %#v", statusByTitle)
	}
	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if !hasRenderedText(records, "Project run paused after 1 task(s)") {
		t.Fatalf("expected blocked remainder notification, got %#v", records)
	}
}

func TestRunProjectTasksBlocksTasksWaitingOnBlockedDependencies(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	outbox := communication.NewMemoryOutbox()
	commRuntime := communication.NewRuntime(outbox, communication.NewRenderer())
	commRuntime.RegisterChannel("web", communication.NewWebConsoleAdapter("web"))

	service := NewServiceWithComponents(
		sqlite,
		blockedDependencyPlanner{},
		titleBlockingValidator{},
		scheduler.NewReadyScheduler(sqlite),
		executor.NewMinimalExecutor(sqlite),
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseCommunication("web", "session-1", commRuntime)
	project, err := service.CreateProject(ctx, CreateProjectRequest{Name: "Dependent block", Goal: "block dependent task"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := service.PlanProject(ctx, project.ID); err != nil {
		t.Fatalf("plan project: %v", err)
	}
	ran, err := service.RunProjectTasks(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("run project tasks: %v", err)
	}
	if ran != 0 {
		t.Fatalf("expected no runnable tasks, got %d", ran)
	}
	tasks, err := sqlite.ListTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	statusByTitle := map[string]domain.TaskStatus{}
	for _, task := range tasks {
		statusByTitle[task.Title] = task.Status
	}
	if statusByTitle["Blocked prerequisite"] != domain.TaskStatusBlocked {
		t.Fatalf("expected prerequisite blocked, got %#v", statusByTitle)
	}
	if statusByTitle["Dependent task"] != domain.TaskStatusBlocked {
		t.Fatalf("expected dependent task to be reconciled to BLOCKED, got %#v", statusByTitle)
	}
	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if !hasRenderedText(records, "Task blocked by dependency") {
		t.Fatalf("expected dependency block notification, got %#v", records)
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

func TestServiceInjectsTaskAwarenessAndAutoMemoryIntoNextTask(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	recorder := &contextRecordingExecutor{store: sqlite}
	service := NewServiceWithComponents(
		sqlite,
		dependencyPlanner{},
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		recorder,
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseContextAssets(nil, nil, nil, memory.NewIndex(), contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), nil)
	project, err := service.CreateProject(ctx, CreateProjectRequest{
		Name: "W9",
		Goal: "Run tasks with task awareness",
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
		t.Fatalf("expected first dependency task, got %s", first.Task.Title)
	}
	second, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		t.Fatalf("run second task: %v", err)
	}
	if second.Task.Title != "Write mystery" {
		t.Fatalf("expected second task, got %s", second.Task.Title)
	}
	secondContext := recorder.contexts[second.Task.ID]
	for _, expected := range []string{"\"project_state\"", "\"task_state\"", "\"depends_on\"", "Outline mystery", "\"relevant_memories\"", "Task completed"} {
		if !strings.Contains(secondContext, expected) {
			t.Fatalf("expected second task context to contain %q:\n%s", expected, secondContext)
		}
	}
}

func TestServiceRetriesFailedTaskThroughRuntimeAPI(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	service := NewService(sqlite)
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "retry", Goal: "retry failed task"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Fail once",
		AcceptanceCriteria: []string{"retry moves task back to ready"},
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
	failed, err := sqlite.TransitionTask(ctx, inProgress.ID, domain.TaskStatusFailed, "failed")
	if err != nil {
		t.Fatalf("fail task: %v", err)
	}
	if err := service.RetryTask(ctx, failed.ID); err != nil {
		t.Fatalf("retry task: %v", err)
	}
	retried, err := sqlite.GetTask(ctx, failed.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if retried.Status != domain.TaskStatusReady {
		t.Fatalf("expected ready after retry, got %s", retried.Status)
	}
}

func TestServiceMarksTestingTaskFailedBeforeRepair(t *testing.T) {
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
		failingTester{store: sqlite},
		reviewer.NewMinimalReviewer(sqlite),
	)
	project, err := service.CreateProject(ctx, CreateProjectRequest{Name: "repair", Goal: "repair failed tester"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	tasks, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("plan project: %v", err)
	}
	_, err = service.RunNextTask(ctx, project.ID)
	if err == nil {
		t.Fatal("expected tester failure")
	}
	failed, err := sqlite.GetTask(ctx, tasks[0].ID)
	if err != nil {
		t.Fatalf("get original task: %v", err)
	}
	if failed.Status != domain.TaskStatusFailed {
		t.Fatalf("expected original task to be FAILED, got %s", failed.Status)
	}
	allTasks, err := sqlite.ListTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	foundRepair := false
	for _, task := range allTasks {
		if strings.HasPrefix(task.Title, "Fix: ") && task.Status == domain.TaskStatusReady {
			foundRepair = true
		}
	}
	if !foundRepair {
		t.Fatalf("expected ready repair task, got %#v", allTasks)
	}
}

func TestLimitContextTextAppliesRuntimeBudget(t *testing.T) {
	got := limitContextText("abcdefghijklmnopqrstuvwxyz", 18)
	if len(got) != 18 {
		t.Fatalf("expected budgeted text length 18, got %d: %q", len(got), got)
	}
	if got == "abcdefghijklmnopqrstuvwxyz" {
		t.Fatal("expected context text to be truncated")
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

type allBlockedPlanner struct{}

func (p allBlockedPlanner) PlanProject(ctx context.Context, req planner.PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Task{
		{
			ProjectID:            req.Project.ID,
			Title:                "Blocked implementation",
			Description:          "Needs unavailable implementation capability",
			Status:               domain.TaskStatusDraft,
			AcceptanceCriteria:   []string{"implementation is complete"},
			RequiredCapabilities: []string{"blocked"},
		},
	}, nil
}

type partialBlockedPlanner struct{}

func (p partialBlockedPlanner) PlanProject(ctx context.Context, req planner.PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Task{
		{
			ProjectID:          req.Project.ID,
			Title:              "Readable slice",
			Description:        "Collect read-only evidence",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"evidence collected"},
		},
		{
			ProjectID:            req.Project.ID,
			Title:                "Blocked implementation",
			Description:          "Needs unavailable implementation capability",
			Status:               domain.TaskStatusDraft,
			AcceptanceCriteria:   []string{"implementation is complete"},
			RequiredCapabilities: []string{"blocked"},
		},
	}, nil
}

type blockedDependencyPlanner struct{}

func (p blockedDependencyPlanner) PlanProject(ctx context.Context, req planner.PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Task{
		{
			ProjectID:            req.Project.ID,
			Title:                "Blocked prerequisite",
			Description:          "Needs unavailable implementation capability",
			Status:               domain.TaskStatusDraft,
			AcceptanceCriteria:   []string{"blocked prerequisite is complete"},
			RequiredCapabilities: []string{"blocked"},
		},
		{
			ProjectID:          req.Project.ID,
			Title:              "Dependent task",
			Description:        "Depends on the blocked prerequisite",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"dependent task is complete"},
			DependsOn:          []string{"Blocked prerequisite"},
		},
	}, nil
}

type titleBlockingValidator struct{}

func (v titleBlockingValidator) ValidateTask(ctx context.Context, task domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, capabilityName := range task.RequiredCapabilities {
		if capabilityName == "blocked" {
			return errors.New("capability unavailable")
		}
	}
	return validator.NewMinimalTaskValidator().ValidateTask(ctx, task)
}

func hasRenderedText(records []communication.OutboxRecord, text string) bool {
	for _, record := range records {
		if strings.Contains(record.Rendered.Text, text) {
			return true
		}
	}
	return false
}

type contextRecordingExecutor struct {
	store    executor.Store
	contexts map[string]string
}

type failingTester struct {
	store tester.Store
}

func (t failingTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (tester.Result, error) {
	if _, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "failing tester started"); err != nil {
		return tester.Result{}, err
	}
	if _, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "forced-failure",
		Status:    domain.TestStatusFailed,
		Output:    "forced tester failure",
	}); err != nil {
		return tester.Result{}, err
	}
	return tester.Result{}, errors.New("forced tester failure")
}

func (e *contextRecordingExecutor) UseRuntimeContext(projectID string, contextText string) {
	if e.contexts == nil {
		e.contexts = map[string]string{}
	}
	e.contexts[projectID] = contextText
}

func (e *contextRecordingExecutor) Execute(ctx context.Context, task domain.Task) (executor.Result, error) {
	if e.contexts == nil {
		e.contexts = map[string]string{}
	}
	e.contexts[task.ID] = e.contexts[task.ProjectID]
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "recording executor started task")
	if err != nil {
		return executor.Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return executor.Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "executor.summary",
		Summary:     "Recorded runtime context for " + task.Title,
		EvidenceRef: "context://" + task.ID,
	}); err != nil {
		return executor.Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "recording executor completed task")
	if err != nil {
		return executor.Result{}, err
	}
	return executor.Result{Task: implemented, Attempt: attempt}, nil
}
