CREATE TABLE IF NOT EXISTS fragments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    node_subtree TEXT NOT NULL DEFAULT '{}',
    bindings TEXT NOT NULL DEFAULT '[]',
    built_in BIGINT NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
    updated_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')
);

CREATE INDEX IF NOT EXISTS idx_fragments_built_in ON fragments(built_in);
CREATE INDEX IF NOT EXISTS idx_fragments_name ON fragments(name);
