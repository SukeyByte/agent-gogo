package scheduler

import (
	"context"
	"database/sql"

	"github.com/SukeyByte/agent-gogo/internal/domain"
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

// TaskClaimer atomically moves tasks from READY to IN_PROGRESS. Claiming in
// one transaction is what makes concurrent workers safe: two workers can
// never run the same task.
type TaskClaimer interface {
	ClaimTasks(ctx context.Context, taskIDs []string) ([]domain.Task, error)
}

type ClaimingScheduler struct {
	*ReadyScheduler
	claimer TaskClaimer
}

func NewClaimingScheduler(tasks TaskLister, claimer TaskClaimer) *ClaimingScheduler {
	return &ClaimingScheduler{ReadyScheduler: NewReadyScheduler(tasks), claimer: claimer}
}

// ClaimNextReadyTasks returns up to limit ready, dependency-satisfied tasks,
// atomically claimed as IN_PROGRESS. It returns sql.ErrNoRows when nothing
// is claimable right now.
func (s *ClaimingScheduler) ClaimNextReadyTasks(ctx context.Context, projectID string, limit int) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 1
	}
	tasks, err := s.tasks.ListTasksByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	dependencies, err := s.tasks.ListTaskDependenciesByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	statusByID := make(map[string]domain.TaskStatus, len(tasks))
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
	}
	dependenciesByTaskID := map[string][]domain.TaskDependency{}
	for _, dependency := range dependencies {
		dependenciesByTaskID[dependency.TaskID] = append(dependenciesByTaskID[dependency.TaskID], dependency)
	}
	candidates := make([]string, 0, limit)
	for _, task := range tasks {
		if len(candidates) >= limit {
			break
		}
		if task.Status == domain.TaskStatusReady && dependenciesDone(task, dependenciesByTaskID, statusByID) {
			candidates = append(candidates, task.ID)
		}
	}
	if len(candidates) == 0 {
		return nil, sql.ErrNoRows
	}
	claimed, err := s.claimer.ClaimTasks(ctx, candidates)
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, sql.ErrNoRows
	}
	return claimed, nil
}
