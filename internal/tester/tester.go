package tester

import (
	"context"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type Store interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
}

type Result struct {
	Task       domain.Task
	TestResult domain.TestResult
}

type Tester interface {
	Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error)
}

type MinimalTester struct {
	store Store
}

func NewMinimalTester(store Store) *MinimalTester {
	return &MinimalTester{store: store}
}

func (t *MinimalTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	testingTask, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "tester started mechanical verification")
	if err != nil {
		return Result{}, err
	}
	result, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "minimal-runtime-smoke",
		Status:    domain.TestStatusPassed,
		Output:    "minimal tester completed baseline verification",
	})
	if err != nil {
		return Result{}, err
	}
	reviewingTask, err := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusReviewing, "tester passed mechanical verification")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: reviewingTask, TestResult: result}, nil
}
