CREATE TABLE IF NOT EXISTS playground_collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    requests TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
    updated_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')
);

CREATE INDEX IF NOT EXISTS idx_collections_updated_at ON playground_collections(updated_at);
