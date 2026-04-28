# AGENTS.md

## Purpose
Database driver interface and per-backend implementations for 8 supported databases.

## Files
| Path | Description |
|------|-------------|
| internal/db/driver.go | `Driver` interface, `DetectDriver`, `OpenDriver` factory |
| internal/db/schema.go | `DataSourceKind` constants for each backend |
| internal/db/sqlite_driver.go | SQLite implementation (modernc.org/sqlite) |
| internal/db/mysql_driver.go | MySQL implementation |
| internal/db/mariadb_driver.go | MariaDB implementation |
| internal/db/postgres_driver.go | PostgreSQL implementation |
| internal/db/cockroachdb_driver.go | CockroachDB implementation |
| internal/db/mssql_driver.go | MSSQL implementation |
| internal/db/mongo_driver.go | MongoDB implementation (non-SQL) |
| internal/db/redis_driver.go | Redis implementation (non-SQL) |
| internal/db/cassandra_driver.go | Cassandra implementation |
| internal/db/driver_test.go | Tests for DetectDriver, DSN parsing |
| internal/db/*.go | Helper files (record.go, value.go, sqlite.go, mysql.go) |

## Dependencies
- Imports `database/sql` (SQL drivers), `modernc.org/sqlite`, `github.com/go-sql-driver/mysql`, etc.
- Non-SQL drivers (MongoDB, Redis) do not use `database/sql`

## Conventions
- Each backend implements the `Driver` interface
- `DetectDriver` parses DSN scheme; no scheme = SQLite
- MariaDB detection takes priority over MySQL; CockroachDB over PostgreSQL

## Gotchas
- Only directory with tests (driver detection, DSN parsing, `replaceLocalhost`)
- Non-SQL drivers override `LoadTableData` with specialized query executors
- `replaceLocalhost` substitutes "localhost" with "127.0.0.1" to avoid IPv6 issues
