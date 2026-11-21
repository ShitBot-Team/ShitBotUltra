-- High-water marks survive log retention and Core restarts. Never rewind them
-- when a task is disabled, removed, or its schedule is edited.
CREATE TABLE task_cursors (
    task_id TEXT PRIMARY KEY,
    last_due_at INTEGER NOT NULL,
    last_slot TEXT NOT NULL
);

CREATE TABLE task_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    task_name TEXT NOT NULL,
    revision TEXT NOT NULL,
    action_kind TEXT NOT NULL,
    target_id TEXT NOT NULL,
    trigger TEXT NOT NULL CHECK (trigger IN ('scheduled', 'manual')),
    request_key TEXT NOT NULL,
    scheduled_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'unknown', 'skipped', 'canceled')),
    error_code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    UNIQUE (task_id, request_key)
);
CREATE INDEX task_runs_created ON task_runs(created_at DESC, id DESC);
CREATE INDEX task_runs_task_created ON task_runs(task_id, created_at DESC, id DESC);
CREATE INDEX task_runs_unfinished ON task_runs(status) WHERE finished_at IS NULL;
