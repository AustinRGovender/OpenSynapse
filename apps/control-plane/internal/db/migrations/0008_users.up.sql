-- Users table (team mode only)
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'editor',       -- admin, editor, viewer
    password_hash TEXT NOT NULL DEFAULT '',     -- argon2id hash
    api_token_hash TEXT NOT NULL DEFAULT '',    -- hashed API token
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_api_token ON users(api_token_hash);
