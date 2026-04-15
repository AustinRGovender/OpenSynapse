-- SQLite does not support DROP COLUMN directly in older versions.
-- For newer SQLite (3.35.0+), this works:
ALTER TABLE crawls DROP COLUMN engine;
