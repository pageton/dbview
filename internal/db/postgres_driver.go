package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

// PostgreSQLDriver implements Driver for PostgreSQL databases.
type PostgreSQLDriver struct {
	db *sql.DB
}

func (d *PostgreSQLDriver) Open(ctx context.Context, dsn string) error {
	// Accept both postgres:// and postgresql:// prefixes
	if strings.HasPrefix(dsn, "postgresql://") {
		dsn = "postgres://" + strings.TrimPrefix(dsn, "postgresql://")
	}
	var err error
	d.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	return d.db.PingContext(ctx)
}

func (d *PostgreSQLDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *PostgreSQLDriver) Kind() DataSourceKind { return KindPostgreSQL }

func (d *PostgreSQLDriver) ListTables(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
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

func (d *PostgreSQLDriver) LoadSchema(ctx context.Context, table string) ([]ColInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT ordinal_position, column_name, data_type, is_nullable,
		        CASE WHEN pk.contype = 'p' THEN true ELSE false END AS is_pk,
		        column_default
		 FROM information_schema.columns c
		 LEFT JOIN pg_constraint pk ON pk.conname = (
		   SELECT conname FROM pg_constraint
		   WHERE conrelid = ($1 || '.' || $2)::regclass AND contype = 'p'
		   LIMIT 1
		 )
		 WHERE c.table_schema = 'public' AND c.table_name = $2
		 ORDER BY c.ordinal_position`, "public", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []ColInfo
	for rows.Next() {
		var c ColInfo
		var ordinal int
		var nullable string
		var isPK bool
		if rows.Scan(&ordinal, &c.Name, &c.Type, &nullable, &isPK, &c.Dflt) == nil {
			c.NotNull = nullable == "NO"
			c.PK = isPK
			c.CID = ordinal - 1
			cols = append(cols, c)
		}
	}
	return cols, nil
}

func (d *PostgreSQLDriver) LoadFKs(ctx context.Context, table string) ([]FKInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT tc.constraint_name, ccu.table_name AS foreign_table,
		        kcu.column_name, ccu.column_name AS foreign_column
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		 JOIN information_schema.constraint_column_usage ccu
		   ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
		 WHERE tc.constraint_type = 'FOREIGN KEY'
		   AND tc.table_schema = 'public'
		   AND tc.table_name = $1`, table)
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

func (d *PostgreSQLDriver) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *PostgreSQLDriver) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

func (d *PostgreSQLDriver) Placeholder(idx int) string {
	return fmt.Sprintf("$%d", idx)
}

func (d *PostgreSQLDriver) QuoteIdent(name string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
}

func (d *PostgreSQLDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *PostgreSQLDriver) DB() *sql.DB { return d.db }

func (d *PostgreSQLDriver) LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error) {
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

func (d *PostgreSQLDriver) RowCount(ctx context.Context, table string) (int, error) {
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

func (d *PostgreSQLDriver) LoadIndices(ctx context.Context, table string) ([]IndexInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT i.relname AS index_name,
		        ix.indisunique AS is_unique,
		        array_agg(a.attname ORDER BY k.n) AS columns
		 FROM pg_class t
		 JOIN pg_index ix ON t.oid = ix.indrelid
		 JOIN pg_class i ON i.oid = ix.indexrelid
		 JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, n) ON true
		 JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		 WHERE t.relname = $1
		   AND t.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
		 GROUP BY i.relname, ix.indisunique
		 ORDER BY i.relname`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var indices []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var colArr string // postgres array like {col1,col2}
		if rows.Scan(&idx.Name, &idx.Unique, &colArr) == nil {
			colArr = strings.Trim(colArr, "{}")
			if colArr != "" {
				idx.Columns = strings.Split(colArr, ",")
			}
			indices = append(indices, idx)
		}
	}
	return indices, nil
}
