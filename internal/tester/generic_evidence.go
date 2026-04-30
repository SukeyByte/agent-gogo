package tester

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type GenericEvidenceStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
}

type GenericEvidenceTester struct {
	store GenericEvidenceStore
}

func NewGenericEvidenceTester(store GenericEvidenceStore) *GenericEvidenceTester {
	return &GenericEvidenceTester{store: store}
}

func (t *GenericEvidenceTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	testingTask, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "generic evidence tester started")
	if err != nil {
		return Result{}, err
	}
	observations, err := t.store.ListObservationsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	calls, err := t.store.ListToolCallsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	if reason := genericEvidenceFailure(task, observations, calls); reason != "" {
		result, createErr := t.store.CreateTestResult(ctx, domain.TestResult{
			AttemptID: attempt.ID,
			Name:      "generic-evidence",
			Status:    domain.TestStatusFailed,
			Output:    reason,
		})
		if createErr != nil {
			return Result{}, createErr
		}
		failedTask, transitionErr := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusFailed, reason)
		if transitionErr != nil {
			return Result{}, transitionErr
		}
		return Result{Task: failedTask, TestResult: result}, errors.New(reason)
	}
	output := fmt.Sprintf("generic evidence passed: observations=%d tool_calls=%d", len(observations), len(calls))
	result, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "generic-evidence",
		Status:    domain.TestStatusPassed,
		Output:    output,
	})
	if err != nil {
		return Result{}, err
	}
	reviewingTask, err := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusReviewing, "generic evidence tester passed")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: reviewingTask, TestResult: result}, nil
}

func genericEvidenceFailure(task domain.Task, observations []domain.Observation, calls []domain.ToolCall) string {
	hasFinish := false
	hasStateEvidence := false
	hasTestRun := false
	hasTestsPassed := false
	successfulCalls := 0
	for _, observation := range observations {
		switch observation.Type {
		case "agent.finish":
			hasFinish = strings.TrimSpace(observation.Summary) != ""
		case "state.file_changed", "state.artifact_written", "state.memory_persisted", "state.tests_passed", "state.command_passed", "state.repository_observed", "state.browser_observed", "state.tool_succeeded":
			hasStateEvidence = strings.TrimSpace(observation.Summary) != ""
		}
		if observation.Type == "state.tests_passed" {
			hasTestRun = true
			hasTestsPassed = true
		}
		if observation.Type == "state.tests_failed" && taskRequiresDiagnosticTestRun(task) {
			hasTestRun = true
			hasStateEvidence = true
		}
	}
	for _, call := range calls {
		if call.Status == domain.ToolCallStatusSucceeded {
			successfulCalls++
		}
		if call.Name == "test.run" {
			hasTestRun = true
			if call.Status == domain.ToolCallStatusSucceeded {
				hasTestsPassed = true
			}
		}
	}
	if len(calls) > 0 && successfulCalls == 0 && !(taskRequiresDiagnosticTestRun(task) && hasTestRun) {
		return "no successful tool calls recorded"
	}
	if !hasFinish {
		return "missing agent.finish observation"
	}
	if !hasStateEvidence {
		return "missing interpreted state evidence from tool execution"
	}
	if taskRequiresPassingTests(task) && !hasTestsPassed {
		return "task acceptance requires tests, but no passing test.run evidence exists"
	}
	if taskRequiresDiagnosticTestRun(task) && !hasTestRun {
		return "task acceptance requires test.run evidence"
	}
	return ""
}

func taskRequiresPassingTests(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if strings.Contains(text, "go test") && (strings.Contains(text, "pass") || strings.Contains(text, "ok") || strings.Contains(text, "通过") || strings.Contains(text, "退出码0") || strings.Contains(text, "status 0")) {
		return true
	}
	for _, marker := range []string{
		"tests pass",
		"tests passed",
		"test passed",
		"passing tests",
		"all tests pass",
		"all tests passed",
		"go test ./... returns ok",
		"returns ok",
		"no fail",
		"验证所有测试",
		"驗證所有測試",
		"所有测试通过",
		"所有測試通過",
		"验证修改成功",
		"確認所有測試",
		"确认所有测试",
		"均通过",
		"全部通过",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskRequiresDiagnosticTestRun(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if taskRequiresPassingTests(task) {
		return false
	}
	for _, marker := range []string{
		"go test",
		"run tests",
		"test output",
		"failing test",
		"failure output",
		"运行失败测试",
		"获取失败",
		"捕获失败",
		"失败输出",
		"执行 go test",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z')
	}) {
		if field == "tests" || field == "testing" {
			return true
		}
	}
	return false
}
