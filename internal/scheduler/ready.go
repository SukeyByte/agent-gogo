package scheduler

import (
	"context"
	"database/sql"

	"github.com/sukeke/agent-gogo/internal/domain"
)

type TaskLister interface {
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	ListTaskDependenciesByProject(ctx context.Context, projectID string) ([]domain.TaskDependency, error)
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
	dependencies, err := s.tasks.ListTaskDependenciesByProject(ctx, projectID)
	if err != nil {
		return domain.Task{}, err
	}
	statusByID := make(map[string]domain.TaskStatus, len(tasks))
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
	}
	dependenciesByTaskID := map[string][]domain.TaskDependency{}
	for _, dependency := range dependencies {
		dependenciesByTaskID[dependency.TaskID] = append(dependenciesByTaskID[dependency.TaskID], dependency)
	}
	for _, task := range tasks {
		if task.Status == domain.TaskStatusReady && dependenciesDone(task, dependenciesByTaskID, statusByID) {
			return task, nil
		}
	}
	return domain.Task{}, sql.ErrNoRows
}

func dependenciesDone(task domain.Task, dependenciesByTaskID map[string][]domain.TaskDependency, statusByID map[string]domain.TaskStatus) bool {
	for _, dependency := range dependenciesByTaskID[task.ID] {
		if statusByID[dependency.DependsOnTaskID] != domain.TaskStatusDone {
			return false
		}
	}
	return true
}
