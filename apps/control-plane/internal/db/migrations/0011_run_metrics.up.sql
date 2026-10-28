-- Per-second aggregated metrics for each run (live charts, reports, comparison).
CREATE TABLE IF NOT EXISTS run_metrics (
    run_id         TEXT    NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    timestamp_ms   INTEGER NOT NULL,
    requests       INTEGER NOT NULL DEFAULT 0,
    failed         INTEGER NOT NULL DEFAULT 0,
    rps            REAL    NOT NULL DEFAULT 0,
    error_rate     REAL    NOT NULL DEFAULT 0,
    p50_ms         REAL    NOT NULL DEFAULT 0,
    p90_ms         REAL    NOT NULL DEFAULT 0,
    p95_ms         REAL    NOT NULL DEFAULT 0,
    p99_ms         REAL    NOT NULL DEFAULT 0,
    active_vus     INTEGER NOT NULL DEFAULT 0,
    bytes_sent     INTEGER NOT NULL DEFAULT 0,
    bytes_received INTEGER NOT NULL DEFAULT 0,
    iterations     INTEGER NOT NULL DEFAULT 0,
    checks_passed  INTEGER NOT NULL DEFAULT 0,
    checks_failed  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, timestamp_ms)
);

-- Recent failed requests per run (the dashboard's error log).
CREATE TABLE IF NOT EXISTS run_errors (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id       TEXT    NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    timestamp_ms INTEGER NOT NULL,
    status       INTEGER NOT NULL DEFAULT 0,
    method       TEXT    NOT NULL DEFAULT '',
    name         TEXT    NOT NULL DEFAULT '',
    url          TEXT    NOT NULL DEFAULT '',
    error_code   TEXT    NOT NULL DEFAULT '',
    message      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_run_errors_run ON run_errors(run_id, timestamp_ms);
