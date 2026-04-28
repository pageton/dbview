# AGENTS.md

## Purpose
Contains the main entry point for the dbview binary.

## Files
| Path | Description |
|------|-------------|
| cmd/dbview/main.go | Entry point: CLI flag parsing, self-update logic, launches Bubble Tea TUI |

## Dependencies
- Imports `github.com/pageton/dbview/internal/app` and `github.com/pageton/dbview/internal/db`
- External: `github.com/charmbracelet/bubbletea`, `net/http` (self-update)

## Conventions
Follows Go `cmd/` package convention: single main package per binary.

## Gotchas
- Self-update uses GitHub Releases API; requires network access.
- Version is injected via `-ldflags "-X main.version=${VERSION}"` at build time.
- Only checks file existence for SQLite DSNs (no scheme prefix).
