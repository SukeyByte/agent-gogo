CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    channel_type TEXT NOT NULL DEFAULT 'cli',
    channel_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    title TEXT NOT NULL DEFAULT '',
    metadata TEXT NOT NULL DEFAULT '{}',
    last_active_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_status ON sessions(user_id, status);
CREATE INDEX IF NOT EXISTS idx_sessions_user_active ON sessions(user_id, status, last_active_at);
CREATE INDEX IF NOT EXISTS idx_sessions_channel ON sessions(channel_type, channel_id);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

CREATE TABLE IF NOT EXISTS session_runtime_contexts (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL DEFAULT '',
    chain_decision TEXT NOT NULL DEFAULT '{}',
    intent_profile TEXT NOT NULL DEFAULT '{}',
    context_text TEXT NOT NULL DEFAULT '',
    memory_snapshot TEXT NOT NULL DEFAULT '[]',
    active_personas TEXT NOT NULL DEFAULT '[]',
    active_skills TEXT NOT NULL DEFAULT '[]',
    frozen_revision TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
