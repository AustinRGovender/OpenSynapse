package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Dialects the migrations are written for.
const (
	dialectSQLite   = "sqlite"
	dialectPostgres = "postgres"
)

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

	if err := migrate(db, dialectSQLite); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return db, nil
}

// OpenPostgres connects to Postgres (cluster mode) and runs migrations. The
// DSN is a libpq URL or key/value string, e.g.
// postgres://user:pass@host:5432/opensynapse?sslmode=disable.
func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Postgres may still be starting next to the control plane; retry briefly.
	for {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			db.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		case <-time.After(time.Second):
		}
	}

	if err := migrate(db, dialectPostgres); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return db, nil
}

// OpenMemory opens a fresh, migrated database for tests: in-memory SQLite,
// or, when OPENSYNAPSE_TEST_POSTGRES_DSN is set, a new schema in that
// Postgres database, so the whole test suite runs against either.
func OpenMemory() (*sql.DB, error) {
	if dsn := os.Getenv("OPENSYNAPSE_TEST_POSTGRES_DSN"); dsn != "" {
		return openPostgresSchema(dsn)
	}
	return Open(":memory:")
}

// openPostgresSchema creates an empty schema and opens a database bound to it.
// The DSN must be a URL (postgres://…).
func openPostgresSchema(dsn string) (*sql.DB, error) {
	admin, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		return nil, err
	}
	defer admin.Close()
	buf := make([]byte, 8)
	rand.Read(buf)
	schema := "test_" + hex.EncodeToString(buf)
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		return nil, fmt.Errorf("create test schema: %w", err)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return OpenPostgres(dsn + sep + "search_path=" + schema)
}

// migrationsTable is created before any migration runs.
var migrationsTable = map[string]string{
	dialectSQLite: `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	dialectPostgres: `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')
	)`,
}

// migrationFiles picks, for each version, the dialect's own file
// (NNNN_name.up.<dialect>.sql) if there is one, else the portable
// NNNN_name.up.sql. Versions are shared, so both dialects stay in step.
func migrationFiles(dialect string) (map[int]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	files := map[int]string{}
	specific := map[int]bool{}
	for _, entry := range entries {
		name := entry.Name()
		own := strings.HasSuffix(name, ".up."+dialect+".sql")
		portable := strings.HasSuffix(name, ".up.sql")
		if !own && !portable {
			continue
		}
		// Parse version number from filename: NNNN_description.up.sql
		var version int
		fmt.Sscanf(name, "%d_", &version)
		if version == 0 || (portable && specific[version]) {
			continue
		}
		files[version] = name
		if own {
			specific[version] = true
		}
	}
	return files, nil
}

func migrate(db *sql.DB, dialect string) error {
	// Create migrations tracking table
	_, err := db.Exec(migrationsTable[dialect])
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

	// Read and apply pending up migrations, in version order.
	files, err := migrationFiles(dialect)
	if err != nil {
		return err
	}
	versions := make([]int, 0, len(files))
	for v := range files {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	for _, version := range versions {
		if applied[version] {
			continue
		}
		content, err := migrationFS.ReadFile("migrations/" + files[version])
		if err != nil {
			return fmt.Errorf("read migration %s: %w", files[version], err)
		}
		if err := applyMigration(db, dialect, version, string(content)); err != nil {
			return err
		}
	}

	return nil
}

// fkOffMarker opts a migration into running with foreign keys disabled,
// which SQLite requires for table rebuilds (otherwise DROP TABLE cascades).
const fkOffMarker = "-- opensynapse:foreign-keys-off"

func applyMigration(db *sql.DB, dialect string, version int, content string) error {
	ctx := context.Background()
	// A dedicated connection: PRAGMA foreign_keys is per-connection and
	// cannot change inside a transaction.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migration %d: %w", version, err)
	}
	defer conn.Close()

	fkOff := dialect == dialectSQLite && strings.HasPrefix(content, fkOffMarker)
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
