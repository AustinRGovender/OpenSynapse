package db

import (
	"context"
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
	// busy_timeout lets concurrent writers (two runs finishing at once) wait
	// for the lock instead of failing immediately with SQLITE_BUSY.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if path == ":memory:" {
		// Every connection to :memory: is a separate database; pin to one so
		// migrations and queries all see the same data.
		db.SetMaxOpenConns(1)
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
	return Open(":memory:")
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

		if err := applyMigration(db, version, string(content)); err != nil {
			return err
		}
	}

	return nil
}

// fkOffMarker opts a migration into running with foreign keys disabled,
// which SQLite requires for table rebuilds (otherwise DROP TABLE cascades).
const fkOffMarker = "-- opensynapse:foreign-keys-off"

func applyMigration(db *sql.DB, version int, content string) error {
	ctx := context.Background()
	// A dedicated connection: PRAGMA foreign_keys is per-connection and
	// cannot change inside a transaction.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migration %d: %w", version, err)
	}
	defer conn.Close()

	fkOff := strings.HasPrefix(content, fkOffMarker)
	if fkOff {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys for migration %d: %w", version, err)
		}
		defer conn.ExecContext(ctx, "PRAGMA foreign_keys=ON")
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for migration %d: %w", version, err)
	}
	if _, err := tx.Exec(content); err != nil {
		tx.Rollback()
		return fmt.Errorf("execute migration %d: %w", version, err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	if fkOff {
		rows, err := tx.Query("PRAGMA foreign_key_check")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("foreign key check for migration %d: %w", version, err)
		}
		bad := rows.Next()
		rows.Close()
		if bad {
			tx.Rollback()
			return fmt.Errorf("migration %d left foreign key violations", version)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}
