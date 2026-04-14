-- Runs table
CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    plan_version INTEGER NOT NULL,
    plan_snapshot TEXT NOT NULL,                 -- full plan JSON
    environment_snapshot TEXT,                    -- full environment JSON
    parameters TEXT NOT NULL DEFAULT '{}',        -- JSON: vus_target, rps_target, duration_seconds, worker_count
    status TEXT NOT NULL DEFAULT 'queued',        -- queued, running, completed, failed, aborted
    started_at TEXT,
    ended_at TEXT,
    summary TEXT,                                 -- JSON RunSummary, populated on completion
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE SET NULL
);

-- Run events table
CREATE TABLE IF NOT EXISTS run_events (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    timestamp_ms INTEGER NOT NULL,
    type TEXT NOT NULL,                           -- start, stop, vu_change, rps_change, duration_change, error, threshold_breach, user_note
    payload TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_runs_plan_id ON runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at);
CREATE INDEX IF NOT EXISTS idx_run_events_run_id ON run_events(run_id);
CREATE INDEX IF NOT EXISTS idx_run_events_timestamp ON run_events(run_id, timestamp_ms);
