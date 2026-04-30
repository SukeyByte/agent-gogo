package reviewer

import (
	"context"
	"errors"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type EvidenceReviewStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListTestResultsByAttempt(ctx context.Context, attemptID string) ([]domain.TestResult, error)
}

type EvidenceReviewer struct {
	store EvidenceReviewStore
}

func NewEvidenceReviewer(store EvidenceReviewStore) *EvidenceReviewer {
	return &EvidenceReviewer{store: store}
}

func (r *EvidenceReviewer) Review(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	observations, err := r.store.ListObservationsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	tests, err := r.store.ListTestResultsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	if !hasPassedTest(tests) {
		return r.reject(ctx, task, attempt, "evidence reviewer requires a passed test result")
	}
	if !hasFinishEvidence(observations) {
		return r.reject(ctx, task, attempt, "evidence reviewer requires agent.finish observation")
	}
	return r.approve(ctx, task, attempt, "evidence reviewer approved task completion")
}

func (r *EvidenceReviewer) reject(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, summary string) (Result, error) {
	result, err := r.store.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID: attempt.ID,
		Status:    domain.ReviewStatusRejected,
		Summary:   summary,
	})
	if err != nil {
		return Result{}, err
	}
	failedTask, err := r.store.TransitionTask(ctx, task.ID, domain.TaskStatusReviewFailed, summary)
	if err != nil {
		return Result{}, err
	}
	failedAttempt, err := r.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusFailed, summary)
	if err != nil {
		return Result{}, err
	}
	return Result{Task: failedTask, Attempt: failedAttempt, ReviewResult: result}, errors.New(summary)
}

func (r *EvidenceReviewer) approve(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, summary string) (Result, error) {
	result, err := r.store.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID: attempt.ID,
		Status:    domain.ReviewStatusApproved,
		Summary:   summary,
	})
	if err != nil {
		return Result{}, err
	}
	doneTask, err := r.store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, summary)
	if err != nil {
		return Result{}, err
	}
	completedAttempt, err := r.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusSucceeded, summary)
	if err != nil {
		return Result{}, err
	}
	return Result{Task: doneTask, Attempt: completedAttempt, ReviewResult: result}, nil
}

func hasPassedTest(tests []domain.TestResult) bool {
	for _, result := range tests {
		if result.Status == domain.TestStatusPassed {
			return true
		}
	}
	return false
}

func hasFinishEvidence(observations []domain.Observation) bool {
	for _, observation := range observations {
		if observation.Type == "agent.finish" && strings.TrimSpace(observation.Summary) != "" {
			return true
		}
	}
	return false
}
