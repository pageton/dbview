# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```sh
# Build
go build -trimpath -ldflags="-s -w" -o dbview ./cmd/dbview

# Run
go run ./cmd/dbview <database-path-or-url>

# Run with local SQLite example
go run ./cmd/dbview ./example.db
```

## Tests & Lint

```sh
# All tests (race detector enabled, matching CI)
go test -v -count=1 -race ./...

# Single test
go test -v -run TestDetectDriver ./internal/db/

# Format check (CI gates on this)
test -z "$(gofmt -l -s .)"

# Vet
go vet ./...
```

CI uses golangci-lint (via `golangci/golangci-lint-action@v7`). Go version: 1.25.

## Nix

This project has a Nix flake. Use `nix develop` for the dev shell (provides `go`, `gopls`, `gotools`). Build with `nix build`. Do not suggest `apt`/`brew`/`dnf`.

## Architecture

**dbview** is a terminal TUI database viewer built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (the Elm architecture: Model-Update-View). It supports 8 database backends: SQLite, MySQL, MariaDB, PostgreSQL, CockroachDB, MSSQL, MongoDB, and Redis.

### Package layout

- `cmd/dbview/main.go` — Entry point. Argument parsing, self-update logic, help text. Creates a `tea.Program` with `app.New(dsn)`.
- `internal/app/` — Core TUI. Contains the Bubble Tea `Model` and all `Update`/`View` logic.
  - `model.go` — Model struct, constructor (`New`), data loading, export/import commands, clipboard helpers.
  - `update.go` — All key/mouse event handling dispatched by `ViewMode`. Includes dialog handlers and view-specific update functions.
  - `view.go` — All rendering. Each `ViewMode` has a `render*` method. Dialog rendering, pagination, help overlay.
  - `messages.go` — Message types (`DataLoaded`, `QueryResult`, `ErrMsg`, etc.), `ViewMode` and `DialogKind` enums.
- `internal/db/` — Database abstraction layer.
  - `driver.go` — `Driver` interface (the unified contract for all backends). `DetectDriver` parses DSN scheme to determine backend. `OpenDriver` factory.
  - `schema.go` — Type definitions: `DataSourceKind`, `ColInfo`, `FKInfo`, `IndexInfo`, `FieldDefinition`, etc.
  - `record.go` — Query log ring buffer, SQL formatting helpers (`FormatSQLInsert/Update/Delete`).
  - `value.go` — Value formatting (JSON pretty-print, binary hex dump, truncation for cell display).
  - `*_driver.go` — One file per backend implementing the `Driver` interface (SQLite, MySQL, MariaDB, PostgreSQL, CockroachDB, MSSQL, MongoDB, Redis).
- `internal/table/` — Table rendering utilities: column width calculation, sorting, filtering, `bubbles/table` construction.
- `internal/theme/` — Theme system with 8 color palettes (dark, light, gruvbox-dark/light, catppuccin-latte/frappe/macchiato/mocha). Style helpers for lipgloss.
- `internal/highlight/` — Syntax highlighter for the query input. Tokenizer with separate keyword sets for SQL, MongoDB, and Redis.

### Key design patterns

- **Bubble Tea Elm architecture**: `Model.Update(msg)` returns `(Model, tea.Cmd)`. Commands are async (data loading, query execution, clipboard). Views are pure string rendering.
- **Driver interface**: All backends implement `Driver` with methods like `Open`, `Close`, `ListTables`, `LoadSchema`, `Query`, `Exec`, `LoadTableData`. Non-SQL drivers (MongoDB, Redis) override `LoadTableData` and have specialized query executors.
- **DSN-based driver detection**: `DetectDriver` inspects the URL scheme (`mysql://`, `mariadb://`, `postgres://`, etc.) to select the driver. No scheme means SQLite (file path).
- **Pagination**: Server-side with `db.PageSize` constant. The model tracks current page and total pages.
- **Dialog overlay**: A `Dialog` struct with `Kind` (confirm, edit, export, add-row, import, confirm-query) overlays any view. Input dialogs use custom cursor-aware text editing (`handleTextInput`).
- **Query log**: Ring buffer (`QueryLog`, max 100 entries) tracks all mutations and queries with timestamps, duration, and error info.

### Adding a new database backend

1. Create `internal/db/<name>_driver.go` implementing the `Driver` interface.
2. Add a `Kind<Name> DataSourceKind` constant in `schema.go`.
3. Add detection logic in `DetectDriver` (`driver.go`).
4. Add the case in `OpenDriver` switch.
5. Add display name in `internal/app/model.go` `viewerTitle()`.
6. Add startup error hints in `internal/app/view.go` `renderStartupError()`.
7. Add driver-specific query detection in `isDestructiveQueryForDriver` (`update.go`).

### Release

Tagged releases (`v*`) trigger `.github/workflows/release.yml` which cross-compiles for linux/amd64 (manylinux2014), linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, and publishes to GitHub Releases with checksums.
