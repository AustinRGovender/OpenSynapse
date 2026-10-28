CREATE TABLE IF NOT EXISTS crawls (
    id TEXT PRIMARY KEY,
    entry_url TEXT NOT NULL,
    auth_config TEXT NOT NULL DEFAULT '{}',
    depth BIGINT NOT NULL DEFAULT 3,
    same_origin BIGINT NOT NULL DEFAULT 1,
    blocklist TEXT NOT NULL DEFAULT '[]',
    request_limit BIGINT NOT NULL DEFAULT 500,
    status TEXT NOT NULL DEFAULT 'pending',
    progress TEXT NOT NULL DEFAULT '{}',
    graph TEXT NOT NULL DEFAULT '{}',
    requests TEXT NOT NULL DEFAULT '[]',
    openapi_url TEXT,
    openapi_spec TEXT,
    generated_plan_id TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
    updated_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')
);

CREATE INDEX IF NOT EXISTS idx_crawls_status ON crawls(status);
CREATE INDEX IF NOT EXISTS idx_crawls_created_at ON crawls(created_at);
