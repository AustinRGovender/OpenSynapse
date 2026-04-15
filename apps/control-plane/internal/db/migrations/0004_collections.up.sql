-- Playground collections
CREATE TABLE IF NOT EXISTS playground_collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    requests TEXT NOT NULL DEFAULT '[]',   -- JSON array of saved request objects
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_collections_updated_at ON playground_collections(updated_at);
