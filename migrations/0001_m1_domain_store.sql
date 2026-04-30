PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    goal TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    acceptance_criteria TEXT NOT NULL DEFAULT '[]',
    required_capabilities TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_project_status ON tasks(project_id, status);

CREATE TABLE IF NOT EXISTS task_dependencies (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    UNIQUE(task_id, depends_on_task_id)
);

CREATE INDEX IF NOT EXISTS idx_task_dependencies_task_id ON task_dependencies(task_id);

CREATE TABLE IF NOT EXISTS task_attempts (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    error TEXT NOT NULL DEFAULT '',
    UNIQUE(task_id, number)
);

CREATE INDEX IF NOT EXISTS idx_task_attempts_task_id ON task_attempts(task_id);

CREATE TABLE IF NOT EXISTS task_events (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    attempt_id TEXT REFERENCES task_attempts(id) ON DELETE SET NULL,
    type TEXT NOT NULL,
    from_state TEXT NOT NULL DEFAULT '',
    to_state TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_task_events_task_id_created_at ON task_events(task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_task_events_attempt_id ON task_events(attempt_id);

CREATE TRIGGER IF NOT EXISTS task_events_prevent_update
BEFORE UPDATE ON task_events
BEGIN
    SELECT RAISE(ABORT, 'task_events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_events_prevent_delete
BEFORE DELETE ON task_events
BEGIN
    SELECT RAISE(ABORT, 'task_events are append-only');
END;

CREATE TABLE IF NOT EXISTS tool_calls (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES task_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    input_json TEXT NOT NULL DEFAULT '{}',
    output_json TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_attempt_id ON tool_calls(attempt_id);

CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES task_attempts(id) ON DELETE CASCADE,
    tool_call_id TEXT REFERENCES tool_calls(id) ON DELETE SET NULL,
    type TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_observations_attempt_id ON observations(attempt_id);

CREATE TABLE IF NOT EXISTS test_results (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES task_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    output TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_test_results_attempt_id ON test_results(attempt_id);

CREATE TABLE IF NOT EXISTS review_results (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES task_attempts(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_review_results_attempt_id ON review_results(attempt_id);

CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    attempt_id TEXT REFERENCES task_attempts(id) ON DELETE SET NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    path TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_artifacts_project_id ON artifacts(project_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_attempt_id ON artifacts(attempt_id);
