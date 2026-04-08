# AGENTS.md

## Build & Run

```sh
go build -trimpath -ldflags="-s -w" -o dbview ./cmd/dbview
go run ./cmd/dbview <database-path-or-url>
go run ./cmd/dbview ./example.db   # local SQLite smoke test
```

## Verify (match CI order)

```sh
gofmt -l -s .                      # must print nothing
go vet ./...
golangci-lint run                  # .golangci.yml: errcheck, govet, staticcheck, unused, ineffassign
go test -v -count=1 -race ./...    # race detector on
```

Run a single test: `go test -v -run TestDetectDriver ./internal/db/`

## Architecture

Bubble Tea TUI (Elm architecture). Single binary, 8 database backends.

- `cmd/dbview/main.go` — entry point, flag parsing, self-update. Creates `tea.NewProgram(app.New(dsn))`.
- `internal/app/` — all TUI logic in 4 large files: `model.go` (~770 lines), `update.go` (~1450 lines), `view.go` (~780 lines), `messages.go`. The `Model` struct holds all state; `Update()` dispatches on `ViewMode`; `View()` renders per-mode.
- `internal/db/` — `Driver` interface (`driver.go`) is the unified contract. One `*_driver.go` per backend. `DetectDriver()` parses DSN scheme; no scheme = SQLite. `OpenDriver()` is the factory.
- `internal/table/`, `internal/theme/`, `internal/highlight/` — rendering utilities.

### Adding a database backend

1. `internal/db/<name>_driver.go` — implement `Driver` interface
2. `internal/db/schema.go` — add `Kind<Name> DataSourceKind` constant
3. `internal/db/driver.go` — add detection in `DetectDriver`, case in `OpenDriver`
4. `internal/app/model.go` — add display name in `viewerTitle()`
5. `internal/app/view.go` — add error hint in `renderStartupError()`
6. `internal/app/update.go` — add driver-specific detection in `isDestructiveQueryForDriver`

## Key constraints

- **Go 1.25** (set in `go.mod` and CI).
- **Nix flake** provides dev shell (`nix develop`). Do not suggest `apt`/`brew`/`dnf`.
- **CGO_ENABLED=0** for all release builds. SQLite uses `modernc.org/sqlite` (pure Go, no CGO).
- **MariaDB detection takes priority over MySQL**: `mariadb://` is checked before `mysql://` in `DetectDriver`. Similarly `cockroachdb://` before `postgres://`.
- **Tests only cover `internal/db/`** (driver detection, DSN parsing, `replaceLocalhost`). No tests for `internal/app/` TUI logic.
- Release builds inject version via `-ldflags "-X main.version=${VERSION}"`.
- Non-SQL drivers (MongoDB, Redis) override `LoadTableData` with specialized query executors — they do not use `database/sql`.
