package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

func (s *SQLiteStore) CreateTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	now := utcNow()
	if task.ID == "" {
		task.ID = newID()
	}
	if task.Status == "" {
		task.Status = domain.TaskStatusDraft
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	criteria, err := json.Marshal(task.AcceptanceCriteria)
	if err != nil {
		return domain.Task{}, err
	}
	requiredCapabilities, err := json.Marshal(task.RequiredCapabilities)
	if err != nil {
		return domain.Task{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, title, description, phase, status, acceptance_criteria, required_capabilities, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.ProjectID, task.Title, task.Description, task.Phase, task.Status, string(criteria), string(requiredCapabilities), formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *SQLiteStore) CreateTaskDependency(ctx context.Context, dependency domain.TaskDependency) (domain.TaskDependency, error) {
	if dependency.ID == "" {
		dependency.ID = newID()
	}
	if dependency.CreatedAt.IsZero() {
		dependency.CreatedAt = utcNow()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_dependencies (id, task_id, depends_on_task_id, created_at)
		VALUES (?, ?, ?, ?)
	`, dependency.ID, dependency.TaskID, dependency.DependsOnTaskID, formatTime(dependency.CreatedAt))
	if err != nil {
		return domain.TaskDependency{}, err
	}
	return dependency, nil
}

func (s *SQLiteStore) GetTask(ctx context.Context, id string) (domain.Task, error) {
	var task domain.Task
	var criteria, requiredCapabilities string
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, title, description, phase, status, acceptance_criteria, required_capabilities, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Phase, &task.Status, &criteria, &requiredCapabilities, &createdAt, &updatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	if err := json.Unmarshal([]byte(criteria), &task.AcceptanceCriteria); err != nil {
		return domain.Task{}, err
	}
	if err := json.Unmarshal([]byte(requiredCapabilities), &task.RequiredCapabilities); err != nil {
		return domain.Task{}, err
	}
	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return domain.Task{}, err
	}
	parsedUpdatedAt, err := parseTime(updatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	task.CreatedAt = parsedCreatedAt
	task.UpdatedAt = parsedUpdatedAt
	return task, nil
}

func (s *SQLiteStore) ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, title, description, phase, status, acceptance_criteria, required_capabilities, created_at, updated_at
		FROM tasks
		WHERE project_id = ?
		ORDER BY created_at, id
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var task domain.Task
		var criteria, requiredCapabilities string
		var createdAt, updatedAt string
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Phase, &task.Status, &criteria, &requiredCapabilities, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(criteria), &task.AcceptanceCriteria); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(requiredCapabilities), &task.RequiredCapabilities); err != nil {
			return nil, err
		}
		task.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		task.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLiteStore) ListTaskDependenciesByProject(ctx context.Context, projectID string) ([]domain.TaskDependency, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dependency.id, dependency.task_id, dependency.depends_on_task_id, dependency.created_at
		FROM task_dependencies dependency
		INNER JOIN tasks task ON task.id = dependency.task_id
		WHERE task.project_id = ?
		ORDER BY dependency.created_at, dependency.id
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dependencies []domain.TaskDependency
	for rows.Next() {
		var dependency domain.TaskDependency
		var createdAt string
		if err := rows.Scan(&dependency.ID, &dependency.TaskID, &dependency.DependsOnTaskID, &createdAt); err != nil {
			return nil, err
		}
		dependency.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

func (s *SQLiteStore) TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, err
	}
	defer rollback(tx)

	task, err := getTaskTx(ctx, tx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := domain.ValidateTaskTransition(task.Status, to); err != nil {
		return domain.Task{}, err
	}

	now := utcNow()
	from := task.Status
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, to, formatTime(now), taskID)
	if err != nil {
		return domain.Task{}, err
	}
	event := domain.TaskEvent{
		ID:        newID(),
		TaskID:    taskID,
		Type:      "task.status_changed",
		FromState: from,
		ToState:   to,
		Message:   message,
		CreatedAt: now,
	}
	if err := insertTaskEventTx(ctx, tx, event); err != nil {
		return domain.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Task{}, err
	}
	task.Status = to
	task.UpdatedAt = now
	return task, nil
}

func (s *SQLiteStore) CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	defer rollback(tx)

	var nextNumber int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(number), 0) + 1
		FROM task_attempts
		WHERE task_id = ?
	`, taskID).Scan(&nextNumber)
	if err != nil {
		return domain.TaskAttempt{}, err
	}

	now := utcNow()
	attempt := domain.TaskAttempt{
		ID:        newID(),
		TaskID:    taskID,
		Number:    nextNumber,
		Status:    domain.AttemptStatusRunning,
		StartedAt: now,
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO task_attempts (id, task_id, number, status, started_at, ended_at, error)
		VALUES (?, ?, ?, ?, ?, NULL, '')
	`, attempt.ID, attempt.TaskID, attempt.Number, attempt.Status, formatTime(attempt.StartedAt))
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	event := domain.TaskEvent{
		ID:        newID(),
		TaskID:    taskID,
		AttemptID: attempt.ID,
		Type:      "task_attempt.created",
		Message:   fmt.Sprintf("created attempt %d", attempt.Number),
		CreatedAt: now,
	}
	if err := insertTaskEventTx(ctx, tx, event); err != nil {
		return domain.TaskAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TaskAttempt{}, err
	}
	return attempt, nil
}

func (s *SQLiteStore) CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	defer rollback(tx)

	attempt, err := getTaskAttemptTx(ctx, tx, attemptID)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	now := utcNow()
	_, err = tx.ExecContext(ctx, `
		UPDATE task_attempts
		SET status = ?, ended_at = ?, error = ?
		WHERE id = ?
	`, status, formatTime(now), errorForAttemptStatus(status, message), attemptID)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	event := domain.TaskEvent{
		ID:        newID(),
		TaskID:    attempt.TaskID,
		AttemptID: attempt.ID,
		Type:      "task_attempt.completed",
		Message:   message,
		CreatedAt: now,
	}
	if err := insertTaskEventTx(ctx, tx, event); err != nil {
		return domain.TaskAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TaskAttempt{}, err
	}
	attempt.Status = status
	attempt.EndedAt = &now
	attempt.Error = errorForAttemptStatus(status, message)
	return attempt, nil
}

func (s *SQLiteStore) GetTaskAttempt(ctx context.Context, attemptID string) (domain.TaskAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	defer rollback(tx)
	return getTaskAttemptTx(ctx, tx, attemptID)
}

func (s *SQLiteStore) ListTaskAttemptsByTask(ctx context.Context, taskID string) ([]domain.TaskAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, number, status, started_at, ended_at, error
		FROM task_attempts
		WHERE task_id = ?
		ORDER BY number, id
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []domain.TaskAttempt
	for rows.Next() {
		attempt, err := scanTaskAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *SQLiteStore) AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error) {
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = utcNow()
	}
	if event.Type == "" {
		return domain.TaskEvent{}, errors.New("task event type is required")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO task_events (id, task_id, attempt_id, type, from_state, to_state, message, payload, created_at)
		VALUES (?, ?, nullif(?, ''), ?, ?, ?, ?, ?, ?)
	`, event.ID, event.TaskID, event.AttemptID, event.Type, event.FromState, event.ToState, event.Message, event.Payload, formatTime(event.CreatedAt)); err != nil {
		return domain.TaskEvent{}, err
	}
	return event, nil
}

func (s *SQLiteStore) ListTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, COALESCE(attempt_id, ''), type, from_state, to_state, message, payload, created_at
		FROM task_events
		WHERE task_id = ?
		ORDER BY created_at, id
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.TaskEvent
	for rows.Next() {
		var event domain.TaskEvent
		var createdAt string
		if err := rows.Scan(&event.ID, &event.TaskID, &event.AttemptID, &event.Type, &event.FromState, &event.ToState, &event.Message, &event.Payload, &createdAt); err != nil {
			return nil, err
		}
		event.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
