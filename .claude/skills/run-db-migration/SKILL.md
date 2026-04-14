---
name: run-db-migration
description: How to create and apply a database schema migration for SQLite and Postgres
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
---

# Database Migrations

## File location

```
apps/control-plane/internal/db/migrations/
  NNNN_short_description.up.sql
  NNNN_short_description.down.sql
```

`NNNN` is a zero-padded sequential number. Check the existing files to find the next number.

## Rules

- Every migration must be reversible. The `.down.sql` must undo the `.up.sql`.
- Use portable SQL where possible. If SQLite and Postgres require different syntax, maintain two files:
  - `NNNN_short_description.up.sqlite.sql`
  - `NNNN_short_description.up.postgres.sql`
  (same for down)
- Do not use `ALTER TABLE ... DROP COLUMN` for SQLite (not supported before 3.35). Recreate the table instead.
- Always include `IF NOT EXISTS` / `IF EXISTS` guards where appropriate.

## Applying migrations

```bash
# Apply all pending migrations
cd apps/control-plane && go run ./cmd/migrate up

# Roll back the last migration
cd apps/control-plane && go run ./cmd/migrate down 1

# Check current migration status
cd apps/control-plane && go run ./cmd/migrate status
```

## After migrating

- Update test fixtures if the migration changes a table referenced in tests
- Commit migration files and fixture updates in the same commit
- If adding a new table, add the corresponding Go struct in `internal/db/models/`

## Reference

- Data model: `docs/04-data-model-and-api.md`
- Persistence architecture: `docs/02-architecture.md` section 7
