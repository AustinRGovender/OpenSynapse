package db

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens or creates a SQLite database at the given path and runs migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return db, nil
}

// OpenMemory opens an in-memory SQLite database with migrations applied.
// MaxOpenConns is set to 1 to ensure all goroutines share the same in-memory database.
func OpenMemory() (*sql.DB, error) {
	db, err := Open(":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func migrate(db *sql.DB) error {
	// Create migrations tracking table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Find applied migrations
	applied := map[int]bool{}
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = true
	}

	// Read and apply pending up migrations
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		// Parse version number from filename: NNNN_description.up.sql
		var version int
		fmt.Sscanf(name, "%d_", &version)
		if version == 0 {
			continue
		}

		if applied[version] {
			continue
		}

		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %d: %w", version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}

	return nil
}
