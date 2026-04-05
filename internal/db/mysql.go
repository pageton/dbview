package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLDriver implements Driver for MySQL databases.
type MySQLDriver struct {
	db *sql.DB
}

func (d *MySQLDriver) Open(ctx context.Context, dsn string) error {
	// Convert mysql://user:pass@host:port/dbname to user:pass@tcp(host:port)/dbname
	dsn = strings.TrimPrefix(dsn, "mysql://")
	// If it doesn't contain "tcp(" already, assume it's a URL-style DSN that needs conversion
	if !strings.Contains(dsn, "tcp(") && strings.Contains(dsn, "@") {
		// Parse: user:pass@host:port/dbname -> user:pass@tcp(host:port)/dbname
		parts := strings.SplitN(dsn, "@", 2)
		if len(parts) == 2 {
			cred := parts[0]
			hostDB := parts[1]
			slashIdx := strings.LastIndex(hostDB, "/")
			var host, dbname string
			if slashIdx >= 0 {
				host = hostDB[:slashIdx]
				dbname = hostDB[slashIdx+1:]
			} else {
				host = hostDB
			}
			dsn = fmt.Sprintf("%s@tcp(%s)/%s", cred, host, dbname)
		}
	}
	var err error
	d.db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	return d.db.PingContext(ctx)
}

func (d *MySQLDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *MySQLDriver) Kind() DataSourceKind { return KindMySQL }

func (d *MySQLDriver) ListTables(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}
	return tables, nil
}

func (d *MySQLDriver) LoadSchema(ctx context.Context, table string) ([]ColInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT ordinal_position, column_name, column_type, is_nullable, column_key, column_default
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ?
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []ColInfo
	for rows.Next() {
		var c ColInfo
		var ordinal int
		var nullable, key string
		if rows.Scan(&ordinal, &c.Name, &c.Type, &nullable, &key, &c.Dflt) == nil {
			c.NotNull = nullable == "NO"
			c.PK = key == "PRI"
			c.CID = ordinal - 1
			cols = append(cols, c)
		}
	}
	return cols, nil
}

func (d *MySQLDriver) LoadFKs(ctx context.Context, table string) ([]FKInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT kcu.constraint_name, kcu.referenced_table_name, kcu.column_name, kcu.referenced_column_name
		 FROM information_schema.key_column_usage kcu
		 WHERE kcu.table_schema = DATABASE()
		   AND kcu.table_name = ?
		   AND kcu.referenced_table_name IS NOT NULL
		 ORDER BY kcu.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fks []FKInfo
	for rows.Next() {
		var f FKInfo
		if rows.Scan(&f.ID, &f.Table, &f.From, &f.To) == nil {
			fks = append(fks, f)
		}
	}
	return fks, nil
}

func (d *MySQLDriver) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *MySQLDriver) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

func (d *MySQLDriver) Placeholder(idx int) string { return "?" }

func (d *MySQLDriver) QuoteIdent(name string) string {
	return fmt.Sprintf("`%s`", strings.ReplaceAll(name, "`", "``"))
}

func (d *MySQLDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *MySQLDriver) DB() *sql.DB { return d.db }

func (d *MySQLDriver) LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error) {
	schema, _ := d.LoadSchema(ctx, table)
	cols = ColNames(schema)
	colExpr := ColSelectExpr(schema)

	r, qerr := d.db.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", d.QuoteIdent(table)))
	if qerr == nil {
		r.Next()
		r.Scan(&total)
		r.Close()
	}

	offset := (page - 1) * pageSize
	q := fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d", colExpr, d.QuoteIdent(table), pageSize, offset)
	r, qerr = d.db.QueryContext(ctx, q)
	if qerr != nil {
		return nil, nil, 0, qerr
	}
	defer r.Close()

	realCols, _ := r.Columns()
	data, _, _ := ScanRows(r, len(realCols))
	return cols, data, total, nil
}

func (d *MySQLDriver) RowCount(ctx context.Context, table string) (int, error) {
	var n int
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", d.QuoteIdent(table)))
	if err != nil {
		return 0, err
	}
	rows.Next()
	rows.Scan(&n)
	rows.Close()
	return n, nil
}

func (d *MySQLDriver) LoadIndices(ctx context.Context, table string) ([]IndexInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT s.index_name, s.non_unique, s.column_name
		 FROM information_schema.statistics s
		 WHERE s.table_schema = DATABASE() AND s.table_name = ?
		 ORDER BY s.index_name, s.seq_in_index`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idxMap := make(map[string]*IndexInfo)
	var order []string
	for rows.Next() {
		var name, colName string
		var nonUnique int
		if rows.Scan(&name, &nonUnique, &colName) != nil {
			continue
		}
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{Name: name, Unique: nonUnique == 0}
			order = append(order, name)
		}
		idxMap[name].Columns = append(idxMap[name].Columns, colName)
	}
	var indices []IndexInfo
	for _, name := range order {
		indices = append(indices, *idxMap[name])
	}
	return indices, nil
}
