package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"dbview/internal/app"
	"dbview/internal/db"
)

var version = "0.1.5"

const releaseAPIURL = "https://api.github.com/repos/pageton/dbview/releases/latest"

type latestRelease struct {
	TagName string `json:"tag_name"`
}

func main() {
	args := os.Args[1:]

	// No arguments — short usage
	if len(args) == 0 {
		fmt.Println("Usage: dbview <database-path-or-url>")
		fmt.Println("Run 'dbview --help' for more information.")
		os.Exit(1)
	}

	// Parse flags
	if len(args) == 2 && args[0] == "update" && args[1] == "--check" {
		if err := checkLatestVersion(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

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
			if arg == "update" {
				fmt.Println("Usage: dbview update --check")
				os.Exit(1)
			}
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

func checkLatestVersion() error {
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dbview")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release check failed: %s", resp.Status)
	}

	var release latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.TagName == "" {
		return fmt.Errorf("latest release did not include a tag")
	}

	current := version
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}

	if release.TagName == current {
		fmt.Printf("dbview %s is up to date\n", current)
		return nil
	}

	fmt.Printf("dbview %s is available (current: %s)\n", release.TagName, current)
	return nil
}

func printHelp() {
	fmt.Print(`dbview - Terminal UI database viewer

USAGE
  dbview <database-path-or-url>
  dbview update --check

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

UPDATE
  update --check   Check for a newer release on GitHub

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
