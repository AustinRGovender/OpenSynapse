CREATE TABLE IF NOT EXISTS run_metrics (
    run_id         TEXT             NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    timestamp_ms   BIGINT           NOT NULL,
    requests       BIGINT           NOT NULL DEFAULT 0,
    failed         BIGINT           NOT NULL DEFAULT 0,
    rps            DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_rate     DOUBLE PRECISION NOT NULL DEFAULT 0,
    p50_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    p90_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    p95_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    p99_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    active_vus     BIGINT           NOT NULL DEFAULT 0,
    bytes_sent     BIGINT           NOT NULL DEFAULT 0,
    bytes_received BIGINT           NOT NULL DEFAULT 0,
    iterations     BIGINT           NOT NULL DEFAULT 0,
    checks_passed  BIGINT           NOT NULL DEFAULT 0,
    checks_failed  BIGINT           NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, timestamp_ms)
);

CREATE TABLE IF NOT EXISTS run_errors (
    id           BIGSERIAL PRIMARY KEY,
    run_id       TEXT   NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    timestamp_ms BIGINT NOT NULL,
    status       BIGINT NOT NULL DEFAULT 0,
    method       TEXT   NOT NULL DEFAULT '',
    name         TEXT   NOT NULL DEFAULT '',
    url          TEXT   NOT NULL DEFAULT '',
    error_code   TEXT   NOT NULL DEFAULT '',
    message      TEXT   NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_run_errors_run ON run_errors(run_id, timestamp_ms);
