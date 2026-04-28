package scheduler

import (
	"context"
	"database/sql"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type TaskLister interface {
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
}

type Scheduler interface {
	NextReadyTask(ctx context.Context, projectID string) (domain.Task, error)
}

type ReadyScheduler struct {
	tasks TaskLister
}

func NewReadyScheduler(tasks TaskLister) *ReadyScheduler {
	return &ReadyScheduler{tasks: tasks}
}

func (s *ReadyScheduler) NextReadyTask(ctx context.Context, projectID string) (domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return domain.Task{}, err
	}
	tasks, err := s.tasks.ListTasksByProject(ctx, projectID)
	if err != nil {
		return domain.Task{}, err
	}
	for _, task := range tasks {
		if task.Status == domain.TaskStatusReady {
			return task, nil
		}
	}
	return domain.Task{}, sql.ErrNoRows
}
