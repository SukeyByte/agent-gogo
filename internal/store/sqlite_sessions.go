package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

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

func (s *SQLiteStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, channel_type, channel_id, project_id, status, title, metadata, last_active_at, created_at, updated_at
		FROM sessions ORDER BY last_active_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
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
