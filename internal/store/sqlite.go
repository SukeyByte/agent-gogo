package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/sukeke/agent-gogo/internal/domain"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	migrations := []string{
		"0001_m1_domain_store.sql",
		"0002_sessions.sql",
	}
	for _, name := range migrations {
		sql, err := loadMigration(name)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *SQLiteStore) CreateProject(ctx context.Context, project domain.Project) (domain.Project, error) {
	now := utcNow()
	if project.ID == "" {
		project.ID = newID()
	}
	if project.Status == "" {
		project.Status = domain.ProjectStatusActive
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	if project.UpdatedAt.IsZero() {
		project.UpdatedAt = project.CreatedAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, goal, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, project.ID, project.Name, project.Goal, project.Status, formatTime(project.CreatedAt), formatTime(project.UpdatedAt))
	if err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (s *SQLiteStore) GetProject(ctx context.Context, id string) (domain.Project, error) {
	var project domain.Project
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, goal, status, created_at, updated_at
		FROM projects
		WHERE id = ?
	`, id).Scan(&project.ID, &project.Name, &project.Goal, &project.Status, &createdAt, &updatedAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (s *SQLiteStore) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, goal, status, created_at, updated_at
		FROM projects
		ORDER BY updated_at DESC, created_at DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var project domain.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&project.ID, &project.Name, &project.Goal, &project.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		project.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		project.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

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

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, title, description, status, acceptance_criteria, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.ProjectID, task.Title, task.Description, task.Status, string(criteria), formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
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
	var criteria string
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, title, description, status, acceptance_criteria, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &criteria, &createdAt, &updatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	if err := json.Unmarshal([]byte(criteria), &task.AcceptanceCriteria); err != nil {
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
		SELECT id, project_id, title, description, status, acceptance_criteria, created_at, updated_at
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
		var criteria string
		var createdAt, updatedAt string
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &criteria, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(criteria), &task.AcceptanceCriteria); err != nil {
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

func (s *SQLiteStore) CreateToolCall(ctx context.Context, call domain.ToolCall) (domain.ToolCall, error) {
	now := utcNow()
	if call.ID == "" {
		call.ID = newID()
	}
	if call.Status == "" {
		call.Status = domain.ToolCallStatusPending
	}
	if call.InputJSON == "" {
		call.InputJSON = "{}"
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = now
	}
	if call.UpdatedAt.IsZero() {
		call.UpdatedAt = call.CreatedAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_calls (id, attempt_id, name, input_json, output_json, status, error, evidence_ref, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, call.ID, call.AttemptID, call.Name, call.InputJSON, call.OutputJSON, call.Status, call.Error, call.EvidenceRef, formatTime(call.CreatedAt), formatTime(call.UpdatedAt))
	if err != nil {
		return domain.ToolCall{}, err
	}
	return call, nil
}

func (s *SQLiteStore) ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, attempt_id, name, input_json, output_json, status, error, evidence_ref, created_at, updated_at
		FROM tool_calls
		WHERE attempt_id = ?
		ORDER BY created_at, id
	`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []domain.ToolCall
	for rows.Next() {
		var call domain.ToolCall
		var createdAt, updatedAt string
		if err := rows.Scan(&call.ID, &call.AttemptID, &call.Name, &call.InputJSON, &call.OutputJSON, &call.Status, &call.Error, &call.EvidenceRef, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		parsedUpdatedAt, err := parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		call.CreatedAt = parsedCreatedAt
		call.UpdatedAt = parsedUpdatedAt
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func (s *SQLiteStore) CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error) {
	if observation.ID == "" {
		observation.ID = newID()
	}
	if observation.CreatedAt.IsZero() {
		observation.CreatedAt = utcNow()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO observations (id, attempt_id, tool_call_id, type, summary, evidence_ref, payload, created_at)
		VALUES (?, ?, nullif(?, ''), ?, ?, ?, ?, ?)
	`, observation.ID, observation.AttemptID, observation.ToolCallID, observation.Type, observation.Summary, observation.EvidenceRef, observation.Payload, formatTime(observation.CreatedAt))
	if err != nil {
		return domain.Observation{}, err
	}
	return observation, nil
}

func (s *SQLiteStore) ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, attempt_id, COALESCE(tool_call_id, ''), type, summary, evidence_ref, payload, created_at
		FROM observations
		WHERE attempt_id = ?
		ORDER BY created_at, id
	`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []domain.Observation
	for rows.Next() {
		var observation domain.Observation
		var createdAt string
		if err := rows.Scan(
			&observation.ID,
			&observation.AttemptID,
			&observation.ToolCallID,
			&observation.Type,
			&observation.Summary,
			&observation.EvidenceRef,
			&observation.Payload,
			&createdAt,
		); err != nil {
			return nil, err
		}
		observation.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func (s *SQLiteStore) CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error) {
	if result.ID == "" {
		result.ID = newID()
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = utcNow()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO test_results (id, attempt_id, name, status, output, evidence_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, result.ID, result.AttemptID, result.Name, result.Status, result.Output, result.EvidenceRef, formatTime(result.CreatedAt))
	if err != nil {
		return domain.TestResult{}, err
	}
	return result, nil
}

func (s *SQLiteStore) ListTestResultsByAttempt(ctx context.Context, attemptID string) ([]domain.TestResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, attempt_id, name, status, output, evidence_ref, created_at
		FROM test_results
		WHERE attempt_id = ?
		ORDER BY created_at, id
	`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.TestResult
	for rows.Next() {
		var result domain.TestResult
		var createdAt string
		if err := rows.Scan(&result.ID, &result.AttemptID, &result.Name, &result.Status, &result.Output, &result.EvidenceRef, &createdAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		result.CreatedAt = parsedCreatedAt
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error) {
	if result.ID == "" {
		result.ID = newID()
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = utcNow()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO review_results (id, attempt_id, status, summary, evidence_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, result.ID, result.AttemptID, result.Status, result.Summary, result.EvidenceRef, formatTime(result.CreatedAt))
	if err != nil {
		return domain.ReviewResult{}, err
	}
	return result, nil
}

func (s *SQLiteStore) ListReviewResultsByAttempt(ctx context.Context, attemptID string) ([]domain.ReviewResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, attempt_id, status, summary, evidence_ref, created_at
		FROM review_results
		WHERE attempt_id = ?
		ORDER BY created_at, id
	`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.ReviewResult
	for rows.Next() {
		var result domain.ReviewResult
		var createdAt string
		if err := rows.Scan(&result.ID, &result.AttemptID, &result.Status, &result.Summary, &result.EvidenceRef, &createdAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		result.CreatedAt = parsedCreatedAt
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) CreateArtifact(ctx context.Context, artifact domain.Artifact) (domain.Artifact, error) {
	if artifact.ID == "" {
		artifact.ID = newID()
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = utcNow()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts (id, attempt_id, project_id, type, path, description, created_at)
		VALUES (?, nullif(?, ''), nullif(?, ''), ?, ?, ?, ?)
	`, artifact.ID, artifact.AttemptID, artifact.ProjectID, artifact.Type, artifact.Path, artifact.Description, formatTime(artifact.CreatedAt))
	if err != nil {
		return domain.Artifact{}, err
	}
	return artifact, nil
}

func (s *SQLiteStore) ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(attempt_id, ''), COALESCE(project_id, ''), type, path, description, created_at
		FROM artifacts
		WHERE project_id = ?
		ORDER BY created_at, id
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []domain.Artifact
	for rows.Next() {
		var artifact domain.Artifact
		var createdAt string
		if err := rows.Scan(&artifact.ID, &artifact.AttemptID, &artifact.ProjectID, &artifact.Type, &artifact.Path, &artifact.Description, &createdAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		artifact.CreatedAt = parsedCreatedAt
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func getTaskTx(ctx context.Context, tx *sql.Tx, id string) (domain.Task, error) {
	var task domain.Task
	var criteria string
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `
		SELECT id, project_id, title, description, status, acceptance_criteria, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &criteria, &createdAt, &updatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	if err := json.Unmarshal([]byte(criteria), &task.AcceptanceCriteria); err != nil {
		return domain.Task{}, err
	}
	task.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Task{}, err
	}
	task.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func getTaskAttemptTx(ctx context.Context, tx *sql.Tx, id string) (domain.TaskAttempt, error) {
	return scanTaskAttempt(tx.QueryRowContext(ctx, `
		SELECT id, task_id, number, status, started_at, ended_at, error
		FROM task_attempts
		WHERE id = ?
	`, id))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTaskAttempt(row scanner) (domain.TaskAttempt, error) {
	var attempt domain.TaskAttempt
	var startedAt string
	var endedAt sql.NullString
	if err := row.Scan(&attempt.ID, &attempt.TaskID, &attempt.Number, &attempt.Status, &startedAt, &endedAt, &attempt.Error); err != nil {
		return domain.TaskAttempt{}, err
	}
	parsedStartedAt, err := parseTime(startedAt)
	if err != nil {
		return domain.TaskAttempt{}, err
	}
	attempt.StartedAt = parsedStartedAt
	if endedAt.Valid {
		parsedEndedAt, err := parseTime(endedAt.String)
		if err != nil {
			return domain.TaskAttempt{}, err
		}
		attempt.EndedAt = &parsedEndedAt
	}
	return attempt, nil
}

func insertTaskEventTx(ctx context.Context, tx *sql.Tx, event domain.TaskEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (id, task_id, attempt_id, type, from_state, to_state, message, payload, created_at)
		VALUES (?, ?, nullif(?, ''), ?, ?, ?, ?, ?, ?)
	`, event.ID, event.TaskID, event.AttemptID, event.Type, event.FromState, event.ToState, event.Message, event.Payload, formatTime(event.CreatedAt))
	return err
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func newID() string {
	return uuid.NewString()
}

func utcNow() time.Time {
	return time.Now().UTC().Round(time.Microsecond)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func errorForAttemptStatus(status domain.AttemptStatus, message string) string {
	if status == domain.AttemptStatusFailed {
		return message
	}
	return ""
}

// --- Session CRUD ---

func (s *SQLiteStore) CreateSession(ctx context.Context, session domain.Session) (domain.Session, error) {
	now := utcNow()
	if session.ID == "" {
		session.ID = newID()
	}
	if session.Status == "" {
		session.Status = domain.SessionStatusActive
	}
	if session.Metadata == "" {
		session.Metadata = "{}"
	}
	if session.LastActiveAt.IsZero() {
		session.LastActiveAt = now
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.UserID, session.ChannelType, session.ChannelID, session.ProjectID,
		session.Status, session.Title, session.Metadata,
		formatTime(session.LastActiveAt), formatTime(session.CreatedAt), formatTime(session.UpdatedAt))
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (domain.Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
		FROM sessions WHERE id = ?
	`, id))
}

func (s *SQLiteStore) UpdateSession(ctx context.Context, session domain.Session) (domain.Session, error) {
	session.UpdatedAt = utcNow()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET user_id = ?, channel_type = ?, channel_id = ?, project_id = ?,
			status = ?, title = ?, metadata = ?, last_active_at = ?, updated_at = ?
		WHERE id = ?
	`, session.UserID, session.ChannelType, session.ChannelID, session.ProjectID,
		session.Status, session.Title, session.Metadata,
		formatTime(session.LastActiveAt), formatTime(session.UpdatedAt), session.ID)
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *SQLiteStore) TouchSession(ctx context.Context, id string) error {
	now := utcNow()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET last_active_at = ?, updated_at = ? WHERE id = ?
	`, formatTime(now), formatTime(now), id)
	return err
}

func (s *SQLiteStore) ListSessionsByUser(ctx context.Context, userID string, status domain.SessionStatus) ([]domain.Session, error) {
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
			FROM sessions WHERE user_id = ? ORDER BY last_active_at DESC
		`, userID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
			FROM sessions WHERE user_id = ? AND status = ? ORDER BY last_active_at DESC
		`, userID, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *SQLiteStore) ListSessionsByChannel(ctx context.Context, channelType, channelID string) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
		FROM sessions WHERE channel_type = ? AND channel_id = ? ORDER BY last_active_at DESC
	`, channelType, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *SQLiteStore) FindActiveSessionByChannel(ctx context.Context, channelType, channelID string) (domain.Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
		FROM sessions WHERE channel_type = ? AND channel_id = ? AND status = ? ORDER BY last_active_at DESC LIMIT 1
	`, channelType, channelID, domain.SessionStatusActive))
}

func (s *SQLiteStore) FindActiveSessionByUser(ctx context.Context, userID string) (domain.Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
		FROM sessions WHERE user_id = ? AND status = ? ORDER BY last_active_at DESC LIMIT 1
	`, userID, domain.SessionStatusActive))
}

func (s *SQLiteStore) ExpireIdleSessions(ctx context.Context, maxIdle time.Duration) (int, error) {
	cutoff := utcNow().Add(-maxIdle)
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET status = ?, updated_at = ? WHERE status = ? AND last_active_at < ?
	`, domain.SessionStatusExpired, formatTime(utcNow()), domain.SessionStatusActive, formatTime(cutoff))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// --- Session Runtime Context CRUD ---

func (s *SQLiteStore) SaveSessionRuntimeContext(ctx context.Context, sctx domain.SessionRuntimeContext) error {
	now := utcNow()
	if sctx.CreatedAt.IsZero() {
		sctx.CreatedAt = now
	}
	sctx.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_runtime_contexts (session_id, project_id, chain_decision, intent_profile, context_text, memory_snapshot, active_personas, active_skills, frozen_revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			project_id = excluded.project_id,
			chain_decision = excluded.chain_decision,
			intent_profile = excluded.intent_profile,
			context_text = excluded.context_text,
			memory_snapshot = excluded.memory_snapshot,
			active_personas = excluded.active_personas,
			active_skills = excluded.active_skills,
			frozen_revision = excluded.frozen_revision,
			updated_at = excluded.updated_at
	`, sctx.SessionID, sctx.ProjectID, sctx.ChainDecision, sctx.IntentProfile,
		sctx.ContextText, sctx.MemorySnapshot, sctx.ActivePersonas, sctx.ActiveSkills,
		sctx.FrozenRevision, formatTime(sctx.CreatedAt), formatTime(sctx.UpdatedAt))
	return err
}

func (s *SQLiteStore) GetSessionRuntimeContext(ctx context.Context, sessionID string) (domain.SessionRuntimeContext, error) {
	var sctx domain.SessionRuntimeContext
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, project_id, chain_decision, intent_profile, context_text, memory_snapshot, active_personas, active_skills, frozen_revision, created_at, updated_at
		FROM session_runtime_contexts WHERE session_id = ?
	`, sessionID).Scan(&sctx.SessionID, &sctx.ProjectID, &sctx.ChainDecision, &sctx.IntentProfile,
		&sctx.ContextText, &sctx.MemorySnapshot, &sctx.ActivePersonas, &sctx.ActiveSkills,
		&sctx.FrozenRevision, &createdAt, &updatedAt)
	if err != nil {
		return domain.SessionRuntimeContext{}, err
	}
	sctx.CreatedAt, _ = parseTime(createdAt)
	sctx.UpdatedAt, _ = parseTime(updatedAt)
	return sctx, nil
}

func (s *SQLiteStore) DeleteSessionRuntimeContext(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_runtime_contexts WHERE session_id = ?`, sessionID)
	return err
}

// --- Session scan helpers ---

func scanSession(row scanner) (domain.Session, error) {
	var session domain.Session
	var lastActiveAt, createdAt, updatedAt string
	err := row.Scan(&session.ID, &session.UserID, &session.ChannelType, &session.ChannelID,
		&session.ProjectID, &session.Status, &session.Title, &session.Metadata,
		&lastActiveAt, &createdAt, &updatedAt)
	if err != nil {
		return domain.Session{}, err
	}
	session.LastActiveAt, _ = parseTime(lastActiveAt)
	session.CreatedAt, _ = parseTime(createdAt)
	session.UpdatedAt, _ = parseTime(updatedAt)
	return session, nil
}

func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	var sessions []domain.Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func loadMigration(name string) (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate store source path")
	}
	path := filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
