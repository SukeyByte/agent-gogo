package tester

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/store"
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

func TestGenericEvidenceTesterRequiresDiscoveryToolForResearchTask(t *testing.T) {
	ctx := context.Background()
	sqlite, _, task, attempt := genericEvidenceFixture(t)
	defer sqlite.Close()
	task.Title = "研究上下文与可用资料"
	task.Description = "先读取、搜索或浏览必要资料，确认任务事实。"
	task.AcceptanceCriteria = []string{"已用可用工具收集完成任务所需的事实和上下文"}

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
	if _, err := tester.Test(ctx, task, attempt); err == nil {
		t.Fatal("expected research task without discovery tool to fail")
	}
}

func TestGenericEvidenceTesterAcceptsDiscoveryToolForResearchTask(t *testing.T) {
	ctx := context.Background()
	sqlite, _, task, attempt := genericEvidenceFixture(t)
	defer sqlite.Close()
	task.Title = "研究上下文与可用资料"
	task.Description = "先读取、搜索或浏览必要资料，确认任务事实。"
	task.AcceptanceCriteria = []string{"已用可用工具收集完成任务所需的事实和上下文"}

	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "state.repository_observed", Summary: "indexed repo"}); err != nil {
		t.Fatalf("state observation: %v", err)
	}
	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "agent.finish", Summary: "done"}); err != nil {
		t.Fatalf("finish observation: %v", err)
	}
	if _, err := sqlite.CreateToolCall(ctx, domain.ToolCall{AttemptID: attempt.ID, Name: "code.index", Status: domain.ToolCallStatusSucceeded}); err != nil {
		t.Fatalf("tool call: %v", err)
	}
	tester := NewGenericEvidenceTester(sqlite)
	result, err := tester.Test(ctx, task, attempt)
	if err != nil {
		t.Fatalf("expected research evidence to pass: %v", err)
	}
	if result.TestResult.Status != domain.TestStatusPassed {
		t.Fatalf("expected passed, got %s", result.TestResult.Status)
	}
}

func TestTaskRequiresTestsIgnoresReadingTestFiles(t *testing.T) {
	task := domain.Task{
		Title:              "读取项目结构和测试文件",
		Description:        "读取当前 Go 模块的所有源文件和测试文件，了解项目结构和测试内容",
		AcceptanceCriteria: []string{"已读取所有 .go 源文件和 _test.go 测试文件"},
	}
	if taskRequiresPassingTests(task) || taskRequiresDiagnosticTestRun(task) {
		t.Fatal("reading test files should not require passing test.run evidence")
	}
}

func TestTaskRequiresDiagnosticTestRunAllowsFailureOutputTask(t *testing.T) {
	task := domain.Task{
		Title:              "运行失败测试并分析错误",
		Description:        "运行 go test ./... 获取失败测试的详细输出，分析错误原因。",
		AcceptanceCriteria: []string{"输出包含 FAIL 关键字和详细错误栈的完整测试输出"},
	}
	if !taskRequiresDiagnosticTestRun(task) {
		t.Fatal("expected failing-test diagnostic task to require test.run evidence")
	}
	if taskRequiresPassingTests(task) {
		t.Fatal("diagnostic failure task should not require passing test.run evidence")
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
