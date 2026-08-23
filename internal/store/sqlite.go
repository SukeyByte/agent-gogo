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

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/google/uuid"
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
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
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
		"0003_communication_outbox.sql",
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
	return s.ensureTaskPlanningMetadata(ctx)
}

func (s *SQLiteStore) ensureTaskPlanningMetadata(ctx context.Context) error {
	if err := s.ensureColumn(ctx, "tasks", "phase", "ALTER TABLE tasks ADD COLUMN phase TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return s.ensureColumn(ctx, "tasks", "required_capabilities", "ALTER TABLE tasks ADD COLUMN required_capabilities TEXT NOT NULL DEFAULT '[]'")
}

func (s *SQLiteStore) ensureColumn(ctx context.Context, table string, column string, statement string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, statement)
	return err
}

func getTaskTx(ctx context.Context, tx *sql.Tx, id string) (domain.Task, error) {
	var task domain.Task
	var criteria, requiredCapabilities string
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `
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
