# AGENTS.md

## Role
Terminal database viewer (TUI) supporting SQLite, MySQL, MariaDB, PostgreSQL, CockroachDB, MSSQL, MongoDB, Redis, Cassandra. Single binary built with Go 1.25, uses Bubble Tea (Elm architecture) for TUI. Pure-Go SQLite via modernc.org/sqlite (CGO_ENABLED=0 for releases).

## Architecture
```
dbview/
├── cmd/dbview/          # Entry point, main binary
├── internal/
│   ├── app/             # Bubble Tea TUI logic (model, update, view)
│   ├── db/              # Database driver interface and implementations
│   ├── table/           # Table rendering utilities
│   ├── theme/           # TUI theme definitions
│   └── highlight/       # SQL syntax highlighting
├── .github/             # GitHub workflows (CI)
├── docs/                # Documentation assets (demo gif)
├── flake.nix            # Nix flake config (build, dev shell)
├── go.mod               # Go module dependencies
├── justfile              # Development task runner
└── AGENTS.md            # This file

### Directory Details

- `cmd/dbview/`: Single `main.go` entry point handling CLI flags (`--help`, `--version`, `update`, `update --check`), self-update via GitHub Releases, and TUI launch.
- `internal/app/`: Core TUI logic split into 6 files following Bubble Tea's Elm architecture. Model holds all state; Update dispatches messages; View renders current mode.
- `internal/db/`: Database abstraction layer. `Driver` interface unifies SQL (SQLite, MySQL, MariaDB, PostgreSQL, CockroachDB, MSSQL) and non-SQL (MongoDB, Redis, Cassandra) backends.
- `internal/table/`: Standalone table rendering utilities for Bubble Tea's table component.
- `internal/theme/`: Defines dark/light TUI themes using Lipgloss styles.
- `internal/highlight/`: SQL syntax highlighting for query input.

## Adding a Database Backend

1. `internal/db/<name>_driver.go` — implement `Driver` interface
2. `internal/db/schema.go` — add `Kind<Name> DataSourceKind` constant
3. `internal/db/driver.go` — add detection in `DetectDriver`, case in `OpenDriver`
4. `internal/app/model.go` — add display name in `viewerTitle()`
5. `internal/app/view.go` — add error hint in `renderStartupError()`
6. `internal/app/update.go` — add driver-specific detection in `isDestructiveQueryForDriver`


## Key Files
| Path | Description |
|------|-------------|
| cmd/dbview/main.go | Entry point: flag parsing, self-update, launches TUI |
| internal/app/model.go | Main Bubble Tea Model struct, holds all state (~770 lines) |
| internal/app/update.go | Update function (Elm architecture), dispatches on ViewMode (~1450 lines) |
| internal/app/view.go | View function, renders per ViewMode (~780 lines) |
| internal/app/messages.go | TUI message types |
| internal/db/driver.go | Driver interface, DetectDriver, OpenDriver factory |
| internal/db/schema.go | DataSourceKind constants for each backend |
| internal/table/table.go | Table rendering utilities |
| internal/theme/theme.go | TUI theme definitions |
| internal/highlight/sql.go | SQL syntax highlighting |
| flake.nix | Nix build config, dev shell, release packaging |
| justfile | Development commands (build, test, lint) |
| .golangci.yml | Linter config (errcheck, govet, staticcheck, unused, ineffassign) |

## Module Map
Internal module boundaries:
- `internal/db`: Standalone driver interface. No internal imports. Exports `Driver` interface, `DetectDriver`, `OpenDriver`.
- `internal/app`: Imports `internal/db`, `internal/table`, `internal/theme`. Core TUI logic.
- `internal/table`: Standalone table rendering. No internal imports.
- `internal/theme`: Standalone theme definitions. No internal imports.
- `internal/highlight`: Standalone SQL highlighting. No internal imports.

## Data Flow
1. `main()` parses CLI args (DSN, flags).
2. `DetectDriver(dsn)` determines database type from DSN scheme (mariadb://, mysql://, etc.), defaults to SQLite.
3. `OpenDriver()` creates the appropriate driver and opens the connection.
4. `app.New(dsn)` initializes the Bubble Tea Model with the driver.
5. `tea.NewProgram()` runs the TUI event loop with alt screen.
6. User interactions trigger messages; `Update()` dispatches to handlers, `View()` renders current state.
7. Database operations (list tables, load data) use the `Driver` interface with context timeouts.

## Dependencies
### Internal Module Graph
- `cmd/dbview` → `internal/app`, `internal/db`
- `internal/app` → `internal/db`, `internal/table`, `internal/theme`
- All other internal packages have no internal dependencies.

### External Dependencies (major)
| Package | Version | Purpose |
|---------|----------|---------|
| github.com/charmbracelet/bubbletea | v1.3.10 | TUI framework (Elm architecture) |
| github.com/charmbracelet/bubbles | v1.0.0 | TUI components (table, spinner) |
| github.com/charmbracelet/lipgloss | v1.1.0 | TUI styling |
| modernc.org/sqlite | v1.48.1 | Pure-Go SQLite driver (CGO_ENABLED=0) |
| github.com/go-sql-driver/mysql | v1.9.3 | MySQL/MariaDB driver |
| github.com/lib/pq | v1.12.3 | PostgreSQL driver |
| github.com/microsoft/go-mssqldb | v1.9.8 | MSSQL driver |
| go.mongodb.org/mongo-driver | v1.17.9 | MongoDB driver |
| github.com/redis/go-redis/v9 | v9.18.0 | Redis driver |
| github.com/xuri/excelize/v2 | v2.10.1 | Excel export |

## Conventions
- **Go Version**: 1.25 (set in go.mod, CI, flake.nix)
- **CGO**: `CGO_ENABLED=0` for all release builds (SQLite uses pure-Go modernc.org/sqlite)
- **Nix**: Use `nix develop` for dev shell; no `apt`/`brew` suggestions
- **Commit Style**: Semantic prefixes (feat:, fix:, chore:, refactor:, docs:, test:, perf:), imperative subject <=72 chars
- **Linting**: `golangci-lint` with errcheck, govet, staticcheck, unused, ineffassign
- **Testing**: Only `internal/db/` has tests (driver detection, DSN parsing, replaceLocalhost). No TUI tests.
- **Self-Update**: Binary releases via GitHub Releases, self-update checks GitHub API.

## CI/CD

- Triggered on push/PR to `main` for `**.go`, `go.mod`, `go.sum` changes.
- Runs `go mod verify`, `go mod download`, build cache warm, `gofmt`, `go vet`, `golangci-lint`, `go test -race`.
- Build job verifies `go build -trimpath -ldflags "-s -w" ./cmd/dbview`.
- Go version 1.25 (matches go.mod).


## Build & Test
Commands from justfile, matching CI order:
```sh
# Build
go build -trimpath -ldflags="-s -w" -o dbview ./cmd/dbview

# Run
go run ./cmd/dbview <database-path-or-url>
go run ./cmd/dbview ./example.db   # SQLite smoke test

# Verify (CI order)
gofmt -l -s .                      # must print nothing
go vet ./...
golangci-lint run
go test -v -count=1 -race ./...

# Single test
go test -v -run TestDetectDriver ./internal/db/
```

## Gotchas

- **Driver Detection Priority**: MariaDB (`mariadb://`) checked before MySQL (`mysql://`); CockroachDB (`cockroach://`) before PostgreSQL (`postgres://`).
- **Untested Areas**: All `internal/app/` TUI logic has no tests.
- **Non-SQL Drivers**: MongoDB and Redis override `LoadTableData` with specialized executors; do not use `database/sql`.
- **SQLite File Check**: Only SQLite (no DSN scheme) checks if the file exists at startup.
- **Self-Update**: Requires network access to GitHub API; binary replaced atomically (with backup).
- **CGO_ENABLED=0**: All release builds disable CGO; SQLite uses pure-Go modernc.org/sqlite.
- **Test Coverage**: Only `internal/db/` has tests; TUI, table, theme, highlight are untested.
- **Large Files**: `internal/app/update.go` (~1450 lines) is the largest file; consider splitting if modifying.


## Security Considerations

- **Secrets**: No secrets in code. Use repo's `sops` helpers and `/run/secrets` paths for Nix systems.
- **Input Validation**: DSN parsing trusts URI format; driver open validates connection.
- **Network Access**: Self-update and MongoDB/Redis connections require network; no auth built into TUI.
- **Binary Integrity**: Self-update downloads from GitHub Releases (trusted source), replaces binary with temp file + atomic rename.
- **DSN Safety**: DSNs may contain credentials; avoid logging full DSN in production.
- **GitHub API**: Self-update uses unauthenticated requests; rate limits apply for frequent checks.

