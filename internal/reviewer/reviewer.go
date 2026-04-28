package reviewer

import (
	"context"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type Store interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
}

type Result struct {
	Task         domain.Task
	Attempt      domain.TaskAttempt
	ReviewResult domain.ReviewResult
}

type Reviewer interface {
	Review(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error)
}

type MinimalReviewer struct {
	store Store
}

func NewMinimalReviewer(store Store) *MinimalReviewer {
	return &MinimalReviewer{store: store}
}

func (r *MinimalReviewer) Review(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result, err := r.store.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID: attempt.ID,
		Status:    domain.ReviewStatusApproved,
		Summary:   "minimal reviewer completed baseline review",
	})
	if err != nil {
		return Result{}, err
	}
	doneTask, err := r.store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, "reviewer approved task completion")
	if err != nil {
		return Result{}, err
	}
	completedAttempt, err := r.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusSucceeded, "reviewer approved task completion")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: doneTask, Attempt: completedAttempt, ReviewResult: result}, nil
}
