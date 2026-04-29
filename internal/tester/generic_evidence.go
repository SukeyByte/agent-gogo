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
	hasTestsPassed := false
	for _, observation := range observations {
		switch observation.Type {
		case "agent.finish":
			hasFinish = strings.TrimSpace(observation.Summary) != ""
		case "state.file_changed", "state.artifact_written", "state.memory_persisted", "state.tests_passed", "state.command_passed", "state.repository_observed", "state.browser_observed", "state.tool_succeeded":
			hasStateEvidence = strings.TrimSpace(observation.Summary) != ""
		}
		if observation.Type == "state.tests_passed" {
			hasTestsPassed = true
		}
	}
	for _, call := range calls {
		if call.Status == domain.ToolCallStatusFailed {
			return "failed tool call exists: " + call.Name + " " + call.Error
		}
		if call.Name == "test.run" && call.Status == domain.ToolCallStatusSucceeded {
			hasTestsPassed = true
		}
	}
	if !hasFinish {
		return "missing agent.finish observation"
	}
	if !hasStateEvidence {
		return "missing interpreted state evidence from tool execution"
	}
	if taskRequiresTests(task) && !hasTestsPassed {
		return "task acceptance requires tests, but no passing test.run evidence exists"
	}
	return ""
}

func taskRequiresTests(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{"go test", "tests pass", "run tests", "测试", "驗證", "验证"} {
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
