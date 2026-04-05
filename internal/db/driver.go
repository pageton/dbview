package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

// Driver is the unified interface for all database backends.
// Each database type (SQLite, MySQL, PostgreSQL, MongoDB, Redis) implements this.
type Driver interface {
	// Open establishes a connection to the database.
	Open(ctx context.Context, dsn string) error
	// Close releases the connection.
	Close() error
	// Kind returns the DataSourceKind for this driver.
	Kind() DataSourceKind
	// ListTables returns all user table/collection names.
	ListTables(ctx context.Context) ([]string, error)
	// LoadSchema returns column metadata for a table.
	LoadSchema(ctx context.Context, table string) ([]ColInfo, error)
	// LoadFKs returns foreign key info for a table.
	LoadFKs(ctx context.Context, table string) ([]FKInfo, error)
	// Query executes a query that returns rows.
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	// Exec executes a query that doesn't return rows.
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	// Placeholder returns the placeholder for the given positional index.
	Placeholder(idx int) string
	// QuoteIdent quotes a table/column identifier.
	QuoteIdent(name string) string
	// Ping checks the connection.
	Ping(ctx context.Context) error
	// DB returns the underlying *sql.DB (nil for non-SQL drivers).
	DB() *sql.DB
	// LoadTableData returns all data for a table as string rows.
	// This is used for initial data loading and pagination, and works
	// for both SQL and non-SQL drivers.
	LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error)
	// RowCount returns the number of rows in a table.
	RowCount(ctx context.Context, table string) (int, error)
	// LoadIndices returns index information for a table.
	LoadIndices(ctx context.Context, table string) ([]IndexInfo, error)
}

// DetectDriver determines the driver kind and cleaned DSN from a connection string.
func DetectDriver(dsn string) (DataSourceKind, string) {
	// Check for URL-style connection strings
	if strings.HasPrefix(dsn, "mysql://") {
		return KindMySQL, dsn
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return KindPostgreSQL, dsn
	}
	if strings.HasPrefix(dsn, "mongodb://") || strings.HasPrefix(dsn, "mongodb+srv://") {
		return KindMongoDB, dsn
	}
	if strings.HasPrefix(dsn, "redis://") || strings.HasPrefix(dsn, "rediss://") {
		return KindRedis, dsn
	}
	// Default: SQLite (bare file path)
	return KindSQLite, dsn
}

// OpenDriver creates and opens the appropriate driver for the given DSN.
func OpenDriver(ctx context.Context, dsn string) (Driver, error) {
	kind, cleanedDSN := DetectDriver(dsn)
	var d Driver
	switch kind {
	case KindSQLite:
		d = &SQLiteDriver{}
	case KindMySQL:
		d = &MySQLDriver{}
	case KindPostgreSQL:
		d = &PostgreSQLDriver{}
	case KindMongoDB:
		d = &MongoDriver{}
	case KindRedis:
		d = &RedisDriver{}
	default:
		return nil, fmt.Errorf("unsupported driver: %s", kind)
	}
	if err := d.Open(ctx, cleanedDSN); err != nil {
		return nil, err
	}
	return d, nil
}

// ParseDSN extracts components from a connection string.
func ParseDSN(dsn string) (scheme, host, database string, err error) {
	u, e := url.Parse(dsn)
	if e != nil {
		return "", "", "", fmt.Errorf("parse DSN: %w", e)
	}
	scheme = u.Scheme
	host = u.Host
	database = strings.TrimPrefix(u.Path, "/")
	return
}
