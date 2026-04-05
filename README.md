# dbview

Terminal TUI database viewer for SQLite, MySQL, PostgreSQL, MongoDB, and Redis.

![dbview demo](docs/dbview-demo-preview.gif)

[Direct demo preview file](docs/dbview-demo-preview.gif)

## Install

```sh
go install github.com/pageton/dbview/cmd/dbview@latest
```

Or install the latest release binary to `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/pageton/dbview/main/install.sh | sh
```

```sh
wget -qO- https://raw.githubusercontent.com/pageton/dbview/main/install.sh | sh
```

Optional overrides:

```sh
curl -fsSL https://raw.githubusercontent.com/pageton/dbview/main/install.sh | \
  DBVIEW_VERSION=v0.1.4 DBVIEW_INSTALL_DIR="$HOME/bin" sh
```

Tagged releases also publish prebuilt binaries and archives in GitHub Releases.

Or from the repo root:

```sh
go build -trimpath -ldflags="-s -w" -o dbview ./cmd/dbview
```

## Usage

```sh
dbview <database-path-or-url>
```

Examples:

```sh
dbview ./mydb.db
dbview mysql://user:pass@host:3306/dbname
dbview postgres://user:pass@host:5432/dbname
dbview mongodb://host:27017/dbname
dbview redis://host:6379
```

Flags: `-h`, `--help`, `-v`, `--version`

## Nix

```sh
# Run directly
nix run github:pageton/dbview -- <database-path-or-url>

# Enter dev shell
nix develop
```

## Key Bindings

### Tables View

| Key | Action |
|-----|--------|
| `↑↓` / `jk` | Navigate |
| `enter` | Open table |
| `s` | View schema |
| `r` | Reload tables |
| `x` | Drop table (with confirmation) |
| `D` | Database stats |
| `F` | Flush table (with confirmation) |
| `/` | SQL query |

### Data View

| Key | Action |
|-----|--------|
| `←→` / `hl` | Select column |
| `↑↓` | Scroll rows |
| `1`-`9` | Sort by column N (toggle ASC/DESC) |
| `e` | Edit cell (with confirmation) |
| `x` | Delete row (with confirmation) |
| `d` | Duplicate row |
| `a` | Add row |
| `I` | Import CSV/JSON |
| `E` | Export (CSV/JSON/XLSX/SQL) |
| `c` | Copy cell to clipboard |
| `C` | Copy row to clipboard |
| `[` `]` | Previous/next page |
| `{` `}` | First/last page |
| `ctrl+f` | Live filter |
| `s` | View schema |
| `r` | Reload data |
| `/` | SQL query |

### Schema View

| Key | Action |
|-----|--------|
| `r` | Reload schema |
| `d` | Jump to data view |
| `/` | SQL query |
| `esc` / `s` | Back to tables |

### Query View

| Key | Action |
|-----|--------|
| `↑↓` | Query history |
| `enter` | Execute query |
| `esc` | Back |

### Query Log

| Key | Action |
|-----|--------|
| `↑↓` / `jk` | Navigate entries |
| `enter` | Expand/collapse entry |

### Global

| Key | Action |
|-----|--------|
| `T` | Cycle theme (8 themes) |
| `Q` | Open query log |
| `?` | Help |
| `q` | Quit |
| `esc` | Go back / cancel |
| `ctrl+c` | Force quit |
