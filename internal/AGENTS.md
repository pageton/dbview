# AGENTS.md

## Purpose
All internal packages for dbview, not exported outside the module.

## Subdirectories
| Path | Purpose |
|------|-------------|
| internal/app/ | Bubble Tea TUI logic (model, update, view) |
| internal/db/ | Database driver interface and per-backend implementations |
| internal/table/ | Table rendering utilities |
| internal/theme/ | TUI theme definitions |
| internal/highlight/ | SQL syntax highlighting |

## Dependencies
- `internal/app` imports `internal/db`, `internal/table`, `internal/theme`
- All other internal packages have no internal dependencies

## Conventions
Internal packages are not exported outside the `github.com/pageton/dbview` module.

## Gotchas
- Only `internal/db/` has tests; all other internal packages are untested.
- TUI logic in `internal/app` uses large files (~770-1450 lines per file).
