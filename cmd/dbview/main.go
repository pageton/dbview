package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pageton/dbview/internal/app"
	"github.com/pageton/dbview/internal/db"
)

var version = "0.1.8"

const releaseAPIURL = "https://api.github.com/repos/pageton/dbview/releases/latest"

type latestRelease struct {
	TagName string       `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
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
	if len(args) >= 1 && args[0] == "update" {
		if len(args) == 2 && args[1] == "--check" {
			if err := checkLatestVersion(); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if err := runSelfUpdate(); err != nil {
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
	p := tea.NewProgram(app.New(dbPath), tea.WithAltScreen(), tea.WithContext(ctx))
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

// --- Self-update ---

func runSelfUpdate() error {
	fmt.Println("Checking for updates...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}

	current := version
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}

	if release.TagName == current {
		fmt.Printf("dbview %s is already up to date\n", current)
		return nil
	}

	assetName := buildAssetName(release.TagName)
	var asset *releaseAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("no matching release asset found for %q (looked for %q)", runtime.GOOS+"/"+runtime.GOARCH, assetName)
	}

	fmt.Printf("Updating dbview %s -> %s\n", current, release.TagName)
	fmt.Printf("Downloading %s ...\n", asset.Name)

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), ".dbview-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := downloadWithProgress(asset.URL, asset.Size, tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Atomic-ish replace: rename temp over the original binary.
	backupPath := exePath + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(exePath, backupPath); err != nil {
		return fmt.Errorf("backup old binary: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Rollback
		_ = os.Rename(backupPath, exePath)
		return fmt.Errorf("replace binary: %w", err)
	}
	_ = os.Remove(backupPath)

	fmt.Printf("\nUpdated dbview to %s\n", release.TagName)
	return nil
}

func fetchLatestRelease() (*latestRelease, error) {
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dbview")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("latest release did not include a tag")
	}
	return &release, nil
}

func buildAssetName(tag string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("dbview_%s_%s_%s%s", tag, goos, goarch, ext)
}

func downloadWithProgress(url string, size int64, dst *os.File) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "dbview")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Use ContentLength as fallback if size is unknown
	total := size
	if total <= 0 {
		total = resp.ContentLength
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	lastPrint := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			downloaded += int64(n)
		}

		now := time.Now()
		if now.Sub(lastPrint) >= 100*time.Millisecond || readErr != nil {
			renderProgress(downloaded, total)
			lastPrint = now
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	// Final progress at 100%
	renderProgress(downloaded, total)
	fmt.Println()
	return nil
}

func renderProgress(downloaded, total int64) {
	const width = 40

	pct := float64(0)
	if total > 0 {
		pct = float64(downloaded) / float64(total)
		if pct > 1 {
			pct = 1
		}
	}

	filled := int(pct * width)
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	downloadedStr := formatBytes(downloaded)
	totalStr := "?"
	if total > 0 {
		totalStr = formatBytes(total)
	}
	pctStr := fmt.Sprintf("%5.1f%%", pct*100)

	fmt.Fprintf(os.Stderr, "\r  [%s] %s %s/%s ", bar, pctStr, downloadedStr, totalStr)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
	)
	switch {
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	default:
		return strconv.FormatInt(b, 10) + "B"
	}
}

func printHelp() {
	fmt.Print(`dbview - Terminal UI database viewer

USAGE
  dbview <database-path-or-url>
  dbview update [--check]

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
  update           Download and install the latest release
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
	  M                Toggle mouse capture / terminal select
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
