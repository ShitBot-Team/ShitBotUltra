CREATE TABLE metadata (key TEXT PRIMARY KEY, value INTEGER NOT NULL);
INSERT INTO metadata(key, value) VALUES ('bindings_revision', 0);

CREATE TABLE bindings (
    network_id TEXT NOT NULL,
    player_name TEXT NOT NULL COLLATE BINARY,
    player_uuid TEXT NOT NULL DEFAULT '',
    identity_mode TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(network_id, player_name),
    CHECK ((platform = '' AND scope = '' AND user_id = '') OR (platform <> '' AND scope <> '' AND user_id <> ''))
);
CREATE UNIQUE INDEX binding_uuid ON bindings(network_id, player_uuid) WHERE player_uuid <> '' AND identity_mode <> 'offline';
CREATE INDEX binding_owner ON bindings(network_id, platform, scope, user_id);

CREATE TABLE bind_codes (
    network_id TEXT NOT NULL,
    player_name TEXT NOT NULL COLLATE BINARY,
    player_uuid TEXT NOT NULL,
    identity_mode TEXT NOT NULL,
    code_hash BLOB NOT NULL,
    salt TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(network_id, player_name)
);
CREATE TABLE bind_attempts (
    network_id TEXT NOT NULL,
    actor_key TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY(network_id, actor_key)
);
CREATE INDEX bind_code_expiry ON bind_codes(expires_at);
CREATE INDEX bind_attempt_expiry ON bind_attempts(expires_at);

CREATE TABLE bridge_cursors (
    node_id TEXT NOT NULL,
    boot_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(node_id, boot_id)
);
CREATE TABLE consumed_events (
    source TEXT NOT NULL,
    event_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY(source, event_id)
);
CREATE INDEX consumed_event_expiry ON consumed_events(expires_at);

CREATE TABLE player_stats (
    network_id TEXT NOT NULL,
    player_name TEXT NOT NULL COLLATE BINARY,
    player_uuid TEXT NOT NULL DEFAULT '',
    identity_mode TEXT NOT NULL,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    session_started_at INTEGER,
    last_seen_at INTEGER NOT NULL,
    node_id TEXT NOT NULL,
    boot_id TEXT NOT NULL,
    PRIMARY KEY(network_id, player_name)
);
CREATE TABLE inventory_snapshots (
    network_id TEXT NOT NULL,
    server_id TEXT NOT NULL,
    player_name TEXT NOT NULL COLLATE BINARY,
    player_uuid TEXT NOT NULL,
    identity_mode TEXT NOT NULL,
    captured_at INTEGER NOT NULL,
    format_version INTEGER NOT NULL,
    payload BLOB NOT NULL,
    PRIMARY KEY(network_id, server_id, player_name)
);
CREATE INDEX inventory_expiry ON inventory_snapshots(captured_at);

CREATE TABLE import_jobs (
    id TEXT PRIMARY KEY,
    digest TEXT NOT NULL,
    report BLOB NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX import_digest ON import_jobs(digest);
