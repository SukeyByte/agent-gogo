package tester

import (
	"context"
	"errors"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type EvidenceStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
}

type EvidenceTester struct {
	store EvidenceStore
}

func NewEvidenceTester(store EvidenceStore) *EvidenceTester {
	return &EvidenceTester{store: store}
}

func (t *EvidenceTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	testingTask, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "evidence tester started")
	if err != nil {
		return Result{}, err
	}
	observations, err := t.store.ListObservationsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	if !hasAnswerObservation(observations) {
		_, _ = t.store.CreateTestResult(ctx, domain.TestResult{
			AttemptID: attempt.ID,
			Name:      "evidence-answer-present",
			Status:    domain.TestStatusFailed,
			Output:    "missing llm.answer observation",
		})
		return Result{}, errors.New("missing llm.answer observation")
	}
	result, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "evidence-answer-present",
		Status:    domain.TestStatusPassed,
		Output:    "found browser.open and llm.answer observations",
	})
	if err != nil {
		return Result{}, err
	}
	reviewingTask, err := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusReviewing, "evidence tester passed")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: reviewingTask, TestResult: result}, nil
}

func hasAnswerObservation(observations []domain.Observation) bool {
	hasBrowser := false
	hasAnswer := false
	for _, observation := range observations {
		switch observation.Type {
		case "browser.open":
			hasBrowser = strings.TrimSpace(observation.Summary) != ""
		case "llm.answer":
			hasAnswer = strings.TrimSpace(observation.Summary) != ""
		}
	}
	return hasBrowser && hasAnswer
}
