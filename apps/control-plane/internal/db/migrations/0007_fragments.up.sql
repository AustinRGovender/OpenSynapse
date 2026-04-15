-- Fragments table
CREATE TABLE IF NOT EXISTS fragments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    node_subtree TEXT NOT NULL DEFAULT '{}',
    bindings TEXT NOT NULL DEFAULT '[]',
    built_in INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_fragments_built_in ON fragments(built_in);
CREATE INDEX IF NOT EXISTS idx_fragments_name ON fragments(name);
