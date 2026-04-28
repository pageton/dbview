# AGENTS.md

## Purpose
All Bubble Tea TUI logic for dbview, following Elm architecture.

## Files
| Path | Description |
|------|-------------|
| internal/app/model.go | Main Model struct, holds all state (~770 lines) |
| internal/app/update.go | Update function, dispatches on ViewMode (~1450 lines) |
| internal/app/view.go | View function, renders per ViewMode (~780 lines) |
| internal/app/messages.go | TUI message type definitions |
| internal/app/components.go | Reusable TUI components |
| internal/app/view_help.go | Help view rendering |

## Dependencies
- Imports `github.com/pageton/dbview/internal/db`, `internal/table`, `internal/theme`
- External: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, `github.com/xuri/excelize/v2`

## Conventions
- Elm architecture: Model (state) + Update (messages) + View (render)
- `ViewMode` enum controls which view is rendered
- State is held in a single `Model` struct

## Gotchas
- No tests for TUI logic.
- Large files: consider splitting if modifying.
- Mouse support is toggled via `mouseOn` field.
