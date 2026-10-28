CREATE TABLE IF NOT EXISTS reports (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    run_ids TEXT NOT NULL DEFAULT '[]',
    metrics TEXT NOT NULL DEFAULT '[]',
    normalisation TEXT NOT NULL DEFAULT 'elapsed_time',
    ai_analysis_cached TEXT,
    created_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')
);

CREATE INDEX IF NOT EXISTS idx_reports_created_at ON reports(created_at);
