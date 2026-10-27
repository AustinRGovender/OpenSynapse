-- opensynapse:foreign-keys-off
-- runs.plan_id was NOT NULL while its foreign key said ON DELETE SET NULL,
-- so deleting any plan that had runs failed. Rebuild the table with a
-- nullable plan_id (SQLite cannot alter a column constraint in place); run
-- history keeps the plan snapshot, so runs stay readable after the plan goes.
CREATE TABLE runs_new (
    id TEXT PRIMARY KEY,
    plan_id TEXT,
    plan_version INTEGER NOT NULL,
    plan_snapshot TEXT NOT NULL,
    environment_snapshot TEXT,
    parameters TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued',
    started_at TEXT,
    ended_at TEXT,
    summary TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE SET NULL
);
INSERT INTO runs_new (id, plan_id, plan_version, plan_snapshot, environment_snapshot, parameters, status, started_at, ended_at, summary, created_at)
    SELECT id, plan_id, plan_version, plan_snapshot, environment_snapshot, parameters, status, started_at, ended_at, summary, created_at FROM runs;
DROP TABLE runs;
ALTER TABLE runs_new RENAME TO runs;
CREATE INDEX IF NOT EXISTS idx_runs_plan_id ON runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at);
