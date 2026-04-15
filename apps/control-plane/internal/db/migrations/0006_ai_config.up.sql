-- AI configuration (single row, desktop mode)
CREATE TABLE IF NOT EXISTS ai_config (
    id TEXT PRIMARY KEY DEFAULT 'default',
    provider TEXT NOT NULL DEFAULT '',           -- openai, anthropic, azure_openai
    api_key_encrypted TEXT NOT NULL DEFAULT '',  -- encrypted key (or plaintext for v1 desktop)
    model TEXT NOT NULL DEFAULT '',
    monthly_cap_usd REAL,
    enabled INTEGER NOT NULL DEFAULT 0,
    azure_endpoint TEXT NOT NULL DEFAULT '',     -- Azure OpenAI deployment endpoint
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- AI analysis cache (per run per question)
CREATE TABLE IF NOT EXISTS ai_analyses (
    id TEXT PRIMARY KEY,
    run_id TEXT,
    report_id TEXT,
    question TEXT NOT NULL,
    prompt TEXT NOT NULL,
    response TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ai_analyses_run_id ON ai_analyses(run_id);
CREATE INDEX IF NOT EXISTS idx_ai_analyses_report_id ON ai_analyses(report_id);

-- AI usage tracking
CREATE TABLE IF NOT EXISTS ai_usage (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    cost_usd REAL NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_created_at ON ai_usage(created_at);
