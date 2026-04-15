-- Reports table (saved comparison configurations)
CREATE TABLE IF NOT EXISTS reports (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    run_ids TEXT NOT NULL DEFAULT '[]',          -- JSON array of run UUIDs
    metrics TEXT NOT NULL DEFAULT '[]',          -- JSON array of metric names
    normalisation TEXT NOT NULL DEFAULT 'elapsed_time',  -- elapsed_time | run_progress
    ai_analysis_cached TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_reports_created_at ON reports(created_at);
