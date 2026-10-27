-- Login sessions (team mode). Tokens are stored as SHA-256 hashes.
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

-- Personal API tokens (team mode); each can be revoked on its own.
CREATE TABLE IF NOT EXISTS api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL,
    last_used_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);

-- Application settings as JSON values keyed by name (F-11).
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Who started a run (team mode).
ALTER TABLE runs ADD COLUMN created_by TEXT;
