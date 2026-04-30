package executor

import (
	"context"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type Store interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
}

type Result struct {
	Task    domain.Task
	Attempt domain.TaskAttempt
}

type Executor interface {
	Execute(ctx context.Context, task domain.Task) (Result, error)
}

type MinimalExecutor struct {
	store Store
}

func NewMinimalExecutor(store Store) *MinimalExecutor {
	return &MinimalExecutor{store: store}
}

func (e *MinimalExecutor) Execute(ctx context.Context, task domain.Task) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "executor started task")
	if err != nil {
		return Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID: attempt.ID,
		Type:      "executor.summary",
		Summary:   "minimal executor completed without external side effects",
	}); err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "state.tool_succeeded",
		Summary:     "minimal executor produced deterministic baseline evidence",
		EvidenceRef: "minimal://executor",
	}); err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "agent.finish",
		Summary:     "minimal executor finished baseline task with state evidence",
		EvidenceRef: "minimal://finish",
	}); err != nil {
		return Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "executor produced implementation result")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: implemented, Attempt: attempt}, nil
}
