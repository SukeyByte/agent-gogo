package store

import (
	"context"
	"database/sql"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

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

func (s *SQLiteStore) UpdateProjectStatus(ctx context.Context, projectID string, status domain.ProjectStatus) (domain.Project, error) {
	now := utcNow()
	result, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, status, formatTime(now), projectID)
	if err != nil {
		return domain.Project{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Project{}, err
	}
	if affected == 0 {
		return domain.Project{}, sql.ErrNoRows
	}
	return s.GetProject(ctx, projectID)
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
