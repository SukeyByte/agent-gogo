package store

import (
	"context"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

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
