package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLiteDriver implements Driver for SQLite databases.
type SQLiteDriver struct {
	db *sql.DB
}

func (d *SQLiteDriver) Open(ctx context.Context, dsn string) error {
	var err error
	d.db, err = sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	d.db.SetMaxOpenConns(1)
	return nil
}

func (d *SQLiteDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *SQLiteDriver) Kind() DataSourceKind { return KindSQLite }

func (d *SQLiteDriver) ListTables(ctx context.Context) ([]string, error) {
	return ListTables(d.db)
}

func (d *SQLiteDriver) LoadSchema(ctx context.Context, table string) ([]ColInfo, error) {
	return LoadSchema(ctx, d.db, table), nil
}

func (d *SQLiteDriver) LoadFKs(ctx context.Context, table string) ([]FKInfo, error) {
	return LoadFKs(ctx, d.db, table), nil
}

func (d *SQLiteDriver) LoadIndices(ctx context.Context, table string) ([]IndexInfo, error) {
	return LoadIndices(ctx, d.db, table), nil
}

func (d *SQLiteDriver) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *SQLiteDriver) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

func (d *SQLiteDriver) Placeholder(idx int) string { return "?" }

func (d *SQLiteDriver) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (d *SQLiteDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *SQLiteDriver) DB() *sql.DB { return d.db }

func (d *SQLiteDriver) LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error) {
	// Get column names from schema
	schema := LoadSchema(ctx, d.db, table)
	cols = ColNames(schema)
	colExpr := ColSelectExpr(schema)

	// Count total rows
	r, qerr := d.db.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", d.QuoteIdent(table)))
	if qerr == nil {
		r.Next()
		_ = r.Scan(&total)
		_ = r.Close()
	}

	// Fetch page
	offset := (page - 1) * pageSize
	q := fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d", colExpr, d.QuoteIdent(table), pageSize, offset)
	r, qerr = d.db.QueryContext(ctx, q)
	if qerr != nil {
		return nil, nil, 0, qerr
	}
	defer func() { _ = r.Close() }()

	realCols, _ := r.Columns()
	data, _, _ := ScanRows(r, len(realCols))

	// If no PK, trim the rowid column
	if !HasPK(schema) {
		var trimmed [][]string
		for _, row := range data {
			if len(row) > 1 {
				trimmed = append(trimmed, row[1:])
			}
		}
		data = trimmed
	}

	return cols, data, total, nil
}

func (d *SQLiteDriver) RowCount(ctx context.Context, table string) (int, error) {
	var n int
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", d.QuoteIdent(table)))
	if err != nil {
		return 0, err
	}
	rows.Next()
	_ = rows.Scan(&n)
	_ = rows.Close()
	return n, nil
}
