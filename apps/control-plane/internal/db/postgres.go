package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
)

// The stores are written in SQLite's dialect. In cluster mode they run on
// Postgres through this driver, which wraps pgx and rewrites each statement:
// `?` placeholders become `$1, $2…`, and json_extract(col, '$.key') becomes
// (col::jsonb ->> 'key'). Arguments are adapted to the schema the Postgres
// migrations create, which mirrors SQLite's storage: booleans are BIGINT 0/1
// and JSON is TEXT. Keeping one dialect in the stores means every query is
// exercised by the SQLite test suite and the Postgres one alike.
const postgresDriverName = "opensynapse-postgres"

func init() {
	sql.Register(postgresDriverName, pgDriver{stdlib.GetDefaultDriver()})
}

type pgDriver struct{ inner driver.Driver }

func (d pgDriver) Open(dsn string) (driver.Conn, error) {
	c, err := d.inner.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &pgConn{Conn: c}, nil
}

type pgConn struct{ driver.Conn }

var jsonExtractRe = regexp.MustCompile(`json_extract\(\s*([A-Za-z_][A-Za-z0-9_.]*)\s*,\s*'\$\.([A-Za-z_][A-Za-z0-9_]*)'\s*\)`)

// rebind converts a SQLite-dialect statement for Postgres.
func rebind(query string) string {
	query = jsonExtractRe.ReplaceAllString(query, "($1::jsonb ->> '$2')")
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	inString := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch {
		case ch == '\'':
			inString = !inString // '' inside a literal toggles twice, which is fine
			b.WriteByte(ch)
		case ch == '?' && !inString:
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// adapt maps Go values onto the column types the Postgres schema uses.
func adapt(v any) any {
	switch t := v.(type) {
	case bool:
		if t {
			return int64(1)
		}
		return int64(0)
	case json.RawMessage:
		return string(t)
	case []byte:
		return string(t)
	case int:
		return int64(t)
	}
	return v
}

func (c *pgConn) CheckNamedValue(nv *driver.NamedValue) error {
	nv.Value = adapt(nv.Value)
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	var err error
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return err
}

func (c *pgConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(rebind(query))
}

func (c *pgConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return p.PrepareContext(ctx, rebind(query))
	}
	return c.Conn.Prepare(rebind(query))
}

func (c *pgConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if e, ok := c.Conn.(driver.ExecerContext); ok {
		return e.ExecContext(ctx, rebind(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *pgConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if q, ok := c.Conn.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, rebind(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *pgConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck // fallback for drivers without BeginTx
}

func (c *pgConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *pgConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *pgConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// PostgresSize reports the current database's size in bytes.
func PostgresSize(d *sql.DB) int64 {
	var n int64
	d.QueryRow("SELECT pg_database_size(current_database())").Scan(&n)
	return n
}
