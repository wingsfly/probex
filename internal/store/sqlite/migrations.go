package sqlite

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    target      TEXT NOT NULL,
    probe_type  TEXT NOT NULL,
    interval_ms INTEGER NOT NULL,
    timeout_ms  INTEGER NOT NULL,
    config      TEXT DEFAULT '{}',
    agent_selector TEXT DEFAULT '{}',
    enabled     INTEGER DEFAULT 1,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    labels          TEXT DEFAULT '{}',
    address         TEXT NOT NULL,
    plugins         TEXT DEFAULT '[]',
    status          TEXT DEFAULT 'disconnected',
    last_heartbeat  INTEGER NOT NULL,
    registered_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS probe_results (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    node_id         TEXT DEFAULT '',
    timestamp       INTEGER NOT NULL,
    success         INTEGER NOT NULL,
    latency_ms      REAL DEFAULT 0,
    jitter_ms       REAL,
    packet_loss_pct REAL,
    dns_resolve_ms  REAL,
    tls_handshake_ms REAL,
    status_code     INTEGER,
    download_bps    REAL,
    upload_bps      REAL,
    error           TEXT,
    extra           TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_results_task_time ON probe_results(task_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_results_agent_time ON probe_results(agent_id, timestamp);

CREATE TABLE IF NOT EXISTS reports (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    task_ids          TEXT NOT NULL,
    agent_ids         TEXT DEFAULT '[]',
    time_range_start  INTEGER NOT NULL,
    time_range_end    INTEGER NOT NULL,
    format            TEXT NOT NULL,
    status            TEXT DEFAULT 'pending',
    file_path         TEXT,
    created_at        INTEGER NOT NULL,
    generated_at      INTEGER
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    task_id           TEXT NOT NULL,
    metric            TEXT NOT NULL,
    operator          TEXT NOT NULL,
    threshold         REAL NOT NULL,
    consecutive_count INTEGER DEFAULT 1,
    enabled           INTEGER DEFAULT 1,
    severity          TEXT DEFAULT 'warning',
    webhook_url       TEXT DEFAULT '',
    slack_webhook_url TEXT DEFAULT '',
    state             TEXT DEFAULT 'ok',
    last_triggered_at INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_task ON alert_rules(task_id);

CREATE TABLE IF NOT EXISTS alert_events (
    id        TEXT PRIMARY KEY,
    rule_id   TEXT NOT NULL,
    rule_name TEXT NOT NULL,
    task_id   TEXT NOT NULL,
    metric    TEXT NOT NULL,
    value     REAL NOT NULL,
    threshold REAL NOT NULL,
    severity  TEXT NOT NULL,
    fired_at  INTEGER NOT NULL,
    FOREIGN KEY (rule_id) REFERENCES alert_rules(id)
);

CREATE INDEX IF NOT EXISTS idx_alert_events_rule ON alert_events(rule_id, fired_at);

CREATE TABLE IF NOT EXISTS probes (
    name             TEXT PRIMARY KEY,
    kind             TEXT NOT NULL DEFAULT 'external',
    description      TEXT DEFAULT '',
    parameter_schema TEXT DEFAULT '{}',
    output_schema    TEXT DEFAULT '{}',
    registered_at    INTEGER NOT NULL,
    last_push_at     INTEGER
);

-- Aggregation tables (time-bucketed summaries)
-- Each has identical schema: per task+agent, one row per time bucket
CREATE TABLE IF NOT EXISTS agg_1m (
    task_id       TEXT NOT NULL,
    agent_id      TEXT NOT NULL,
    bucket_start  INTEGER NOT NULL,
    bucket_end    INTEGER NOT NULL,
    count         INTEGER NOT NULL,
    success_count INTEGER NOT NULL,
    metrics       TEXT DEFAULT '{}',
    PRIMARY KEY (task_id, agent_id, bucket_start)
);

CREATE TABLE IF NOT EXISTS agg_10m (
    task_id       TEXT NOT NULL,
    agent_id      TEXT NOT NULL,
    bucket_start  INTEGER NOT NULL,
    bucket_end    INTEGER NOT NULL,
    count         INTEGER NOT NULL,
    success_count INTEGER NOT NULL,
    metrics       TEXT DEFAULT '{}',
    PRIMARY KEY (task_id, agent_id, bucket_start)
);

CREATE TABLE IF NOT EXISTS agg_1h (
    task_id       TEXT NOT NULL,
    agent_id      TEXT NOT NULL,
    bucket_start  INTEGER NOT NULL,
    bucket_end    INTEGER NOT NULL,
    count         INTEGER NOT NULL,
    success_count INTEGER NOT NULL,
    metrics       TEXT DEFAULT '{}',
    PRIMARY KEY (task_id, agent_id, bucket_start)
);

CREATE TABLE IF NOT EXISTS agg_8h (
    task_id       TEXT NOT NULL,
    agent_id      TEXT NOT NULL,
    bucket_start  INTEGER NOT NULL,
    bucket_end    INTEGER NOT NULL,
    count         INTEGER NOT NULL,
    success_count INTEGER NOT NULL,
    metrics       TEXT DEFAULT '{}',
    PRIMARY KEY (task_id, agent_id, bucket_start)
);
`
