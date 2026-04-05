package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"dbview/internal/app"
	"dbview/internal/db"
)

var version = "0.1.0"

func main() {
	args := os.Args[1:]

	// No arguments — short usage
	if len(args) == 0 {
		fmt.Println("Usage: dbview <database-path-or-url>")
		fmt.Println("Run 'dbview --help' for more information.")
		os.Exit(1)
	}

	// Parse flags
	dbPath := ""
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printHelp()
			os.Exit(0)
		case "-v", "--version":
			fmt.Printf("dbview %s\n", version)
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Printf("Unknown flag: %s\n", arg)
				fmt.Println("Run 'dbview --help' for more information.")
				os.Exit(1)
			}
			dbPath = arg
		}
	}

	if dbPath == "" {
		fmt.Println("Usage: dbview <database-path-or-url>")
		fmt.Println("Run 'dbview --help' for more information.")
		os.Exit(1)
	}

	// Only check file existence for SQLite (no scheme prefix)
	kind, _ := db.DetectDriver(dbPath)
	if kind == db.KindSQLite {
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Printf("Error: file not found: %s\n", dbPath)
			os.Exit(1)
		}
	}

	ctx := context.Background()
	p := tea.NewProgram(app.New(dbPath), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`dbview - Terminal UI database viewer

USAGE
  dbview <database-path-or-url>

DATABASES
  SQLite       dbview ./mydb.db
  MySQL        dbview mysql://user:pass@host:3306/dbname
  PostgreSQL   dbview postgres://user:pass@host:5432/dbname
  MongoDB      dbview mongodb://host:27017/dbname
  Redis        dbview redis://host:6379

DESCRIPTION
  Open a database in an interactive terminal UI.
  Browse tables, view and edit data, run queries, and
  export to CSV, JSON, XLSX, or SQL dump.

FLAGS
  -h, --help       Show this help message
  -v, --version    Print version

KEYBINDINGS

  Tables View
    ↑↓ / jk         Navigate tables
    enter            Open table data
    s                View schema
    x                Drop table (with confirmation)
    D                Database stats
    F                Flush table (with confirmation)
    /                Open SQL query

  Data View
    ←→ / hl          Select column
    ↑↓               Scroll rows
    1-9              Sort by column N (toggle ASC/DESC)
    e                Edit cell (with confirmation)
    x                Delete row (with confirmation)
    d                Duplicate row
    a                Add new row
    I                Import CSV/JSON
    E                Export (CSV/JSON/XLSX/SQL)
    c                Copy cell to clipboard
    C                Copy row to clipboard
    [ ]              Previous/next page
    { }              First/last page
    ctrl+f           Live filter
    s                View schema
    /                Open SQL query

  Query View
    ↑↓               Query history
    enter            Execute query
    esc              Back

  Query Log (Q from any view)
    ↑↓               Navigate entries
    enter            Expand/collapse entry

  Global
    T                Cycle theme (8 themes)
    Q                Open query log
    ?                Toggle help overlay
    q                Quit
    esc              Go back / cancel
    ctrl+c           Force quit

DEPENDENCIES
  Clipboard copy (c/C) requires one of:
    xclip              X11 Linux
    wl-copy            Wayland Linux
    pbcopy             macOS
    termux-clipboard-set  Android/Termux

  Export formats CSV, JSON, XLSX, SQL require no extra tools.
  XLSX export needs github.com/xuri/excelize (vendored).
`)
}
