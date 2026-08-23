CREATE TABLE IF NOT EXISTS communication_outbox (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT 'low',
    payload TEXT NOT NULL DEFAULT '{}',
    rendered TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    delivered_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_communication_outbox_channel_created_at ON communication_outbox(channel_id, created_at);
CREATE INDEX IF NOT EXISTS idx_communication_outbox_status_created_at ON communication_outbox(status, created_at);
CREATE INDEX IF NOT EXISTS idx_communication_outbox_project_id ON communication_outbox(project_id);
CREATE INDEX IF NOT EXISTS idx_communication_outbox_session_id ON communication_outbox(session_id);
