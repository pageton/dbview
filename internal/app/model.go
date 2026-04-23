package app

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pageton/dbview/internal/db"
	"github.com/pageton/dbview/internal/table"
	"github.com/pageton/dbview/internal/theme"

	btable "github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xuri/excelize/v2"
)

const startupTimeout = 10 * time.Second

// Model is the main Bubble Tea model for the app.
type Model struct {
	driver        db.Driver
	ctx           context.Context
	cancel        context.CancelFunc
	tables        []string
	schema        map[string][]db.ColInfo
	fks           map[string][]db.FKInfo
	view          ViewMode
	cursor        int
	dbPath        string
	query         string
	queryCursor   int
	searches      []string
	searchInput   string
	searchCursor  int
	searchHist    []string
	sHistIdx      int
	dataTbl       btable.Model
	activeTbl     string
	dataCols      []string
	allRows       [][]string
	totalRows     int
	sortCol       int
	sortAsc       bool
	affected      int64
	err           error
	width         int
	height        int
	ready         bool
	dialog        Dialog
	status        string
	page          int
	pages         int
	colCursor     int
	theme         string
	helpVis       bool
	queryHist     []string
	qHistIdx      int
	spinner       int
	loading       bool
	flash         string
	flashEnd      time.Time
	dbFileSize    string
	dbFileHash    string
	queryLog      db.QueryLog
	prevView      ViewMode
	logCursor     int
	logExpand     bool
	mouseOn       bool
	lastQuitPress time.Time
	detailRows    [][]string
}

// New creates a new Model by opening the given database.
func New(dsn string) Model {
	m := Model{
		dbPath:     dsn,
		view:       ViewTables,
		schema:     make(map[string][]db.ColInfo),
		fks:        make(map[string][]db.FKInfo),
		theme:      "dark",
		width:      80,
		height:     24,
		page:       1,
		qHistIdx:   -1,
		queryHist:  []string{},
		searchHist: []string{},
		dbFileSize: "",
		dbFileHash: "",
		queryLog:   db.NewQueryLog(),
		mouseOn:    false,
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	startupCtx, startupCancel := context.WithTimeout(m.ctx, startupTimeout)
	defer startupCancel()

	driver, err := db.OpenDriver(startupCtx, dsn)
	if err != nil {
		m.err = fmt.Errorf("open db: %w", err)
		return m
	}
	m.driver = driver

	tables, err := driver.ListTables(startupCtx)
	if err != nil {
		m.err = fmt.Errorf("list tables: %w", err)
		return m
	}
	for _, name := range tables {
		m.schema[name], _ = driver.LoadSchema(startupCtx, name)
		m.fks[name], _ = driver.LoadFKs(startupCtx, name)
	}
	m.tables = tables
	m.ready = true
	m.dbFileSize = m.computeDBSize()
	m.dbFileHash = m.computeFileHash()
	return m
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return doTick()
}

func doTick() tea.Cmd {
	return tea.Tick(time.Millisecond*150, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// --- Helper methods ---

// redactDSN masks the password in a connection string for safe display.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn // SQLite path or unparseable — return as-is
	}
	if _, hasPass := u.User.Password(); hasPass {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

func (m Model) setStatus(s string) Model {
	m.status = s
	m.flash = s
	m.flashEnd = time.Now().Add(3 * time.Second)
	return m
}

func (m Model) c() theme.Colors {
	return theme.Resolve(m.theme)
}

func (m Model) colNames(tbl string) []string {
	return db.ColNames(m.schema[tbl])
}

func (m Model) rowCount(tbl string) int {
	n, err := m.driver.RowCount(m.ctx, tbl)
	if err != nil {
		return 0
	}
	return n
}

// visibleRow returns the row data at the given cursor position, respecting
// active search filters and sort order. Falls back to allRows when unfiltered.
func (m Model) visibleRow(cursor int) []string {
	filtered := m.filteredRows()
	if len(filtered) != len(m.allRows) || len(m.searches) > 0 {
		sorted := m.sortedRows(filtered)
		if cursor >= 0 && cursor < len(sorted) {
			return sorted[cursor]
		}
	}
	if cursor >= 0 && cursor < len(m.allRows) {
		return m.allRows[cursor]
	}
	return nil
}

func (m Model) globalRowIdx(cursor int) int {
	return (m.page-1)*db.PageSize + cursor
}

func (m Model) calcPages() int {
	if m.totalRows <= 0 {
		return 1
	}
	p := m.totalRows / db.PageSize
	if m.totalRows%db.PageSize > 0 {
		p++
	}
	return p
}

func (m Model) pkWhere(cursor int) (string, []interface{}) {
	info := m.schema[m.activeTbl]
	var pkCols []string
	for _, c := range info {
		if c.PK {
			pkCols = append(pkCols, c.Name)
		}
	}

	if len(pkCols) == 0 {
		if cursor >= len(m.allRows) {
			return "", nil
		}
		offset := (m.page - 1) * db.PageSize
		rowidQ := fmt.Sprintf("SELECT rowid FROM %q LIMIT 1 OFFSET %d", m.activeTbl, offset+cursor)
		rows, err := m.driver.Query(m.ctx, rowidQ)
		if err != nil {
			return "", nil
		}
		var rowid interface{}
		if !rows.Next() {
			_ = rows.Close()
			return "", nil
		}
		_ = rows.Scan(&rowid)
		_ = rows.Close()
		return "rowid = ?", []interface{}{rowid}
	}

	var conds []string
	var args []interface{}
	for _, pk := range pkCols {
		for i, c := range m.dataCols {
			row := m.visibleRow(cursor)
			if c == pk && row != nil && i < len(row) {
				conds = append(conds, fmt.Sprintf("%q = ?", pk))
				args = append(args, row[i])
			}
		}
	}
	return strings.Join(conds, " AND "), args
}

func (m Model) computeFileHash() string {
	fi, err := os.Stat(m.dbPath)
	if err != nil {
		return "0"
	}
	return fmt.Sprintf("%.1f KB", float64(fi.Size())/1024)
}

func (m Model) computeDBSize() string {
	fi, err := os.Stat(m.dbPath)
	if err != nil {
		return "—"
	}
	sz := float64(fi.Size())
	if sz >= 1048576 {
		return fmt.Sprintf("%.1f MB", sz/1048576)
	}
	if sz >= 1024 {
		return fmt.Sprintf("%.1f KB", sz/1024)
	}
	return fmt.Sprintf("%d B", int(sz))
}

func (m Model) dbSize() string {
	return m.dbFileSize
}

func (m *Model) refreshCachedStats() {
	m.dbFileSize = m.computeDBSize()
	m.dbFileHash = m.computeFileHash()
}

func (m Model) filteredRows() [][]string {
	terms := make([]string, len(m.searches))
	copy(terms, m.searches)
	if m.view == ViewSearch && strings.TrimSpace(m.searchInput) != "" {
		// Split live input on "+" for multi-term live filtering
		parts := strings.Split(m.searchInput, "+")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, "\"'")
			if p != "" {
				terms = append(terms, p)
			}
		}
	}
	return table.FilteredRows(m.allRows, terms)
}

func (m Model) sortedRows(rows [][]string) [][]string {
	return table.SortedRows(rows, m.sortCol, m.sortAsc, len(m.dataCols))
}

func (m Model) buildTable(rows [][]string, cols []string) btable.Model {
	return table.BuildTable(rows, cols, m.width, m.height, m.sortCol, m.sortAsc, m.c())
}

func (m Model) refreshTable() Model {
	rows := m.filteredRows()
	rows = m.sortedRows(rows)
	m.dataTbl = m.buildTable(rows, m.dataCols)
	return m
}

// --- Clipboard ---

// clipboardTools lists clipboard utilities to try, in order of preference.
// Supports: xclip (X11), wl-copy (Wayland), pbcopy (macOS), termux-clipboard-set (Android).
var clipboardTools = []struct {
	cmd  string
	args []string
}{
	{"xclip", []string{"-selection", "clipboard"}},
	{"wl-copy", nil},
	{"pbcopy", nil},
	{"termux-clipboard-set", nil},
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		for _, tool := range clipboardTools {
			path, err := exec.LookPath(tool.cmd)
			if err != nil {
				continue
			}
			cmd := exec.Command(path, tool.args...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return ClipboardMsg{Text: text, OK: true}
			}
		}
		return ClipboardMsg{Text: text, OK: false}
	}
}

// --- Data loading ---

func (m Model) loadData(tbl string) tea.Cmd {
	return func() tea.Msg {
		cols, data, total, err := m.driver.LoadTableData(m.ctx, tbl, 1, db.PageSize)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return DataLoaded{Cols: cols, Rows: data, Total: total, TblName: tbl, Page: 1}
	}
}

func (m Model) loadPage(tbl string, page int) tea.Cmd {
	return func() tea.Msg {
		cols, data, total, err := m.driver.LoadTableData(m.ctx, tbl, page, db.PageSize)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return DataLoaded{Cols: cols, Rows: data, Total: total, TblName: tbl, Page: page}
	}
}

func (m Model) execQuery(q string) tea.Cmd {
	return func() tea.Msg {
		logEntry := db.QueryLogEntry{
			Timestamp: time.Now(),
			Operation: "SELECT",
			Query:     q,
		}

		if m.driver != nil {
			switch m.driver.Kind() {
			case db.KindMongoDB:
				md, ok := m.driver.(*db.MongoDriver)
				if !ok {
					logEntry.Error = fmt.Errorf("mongo driver mismatch")
					logEntry.Duration = time.Since(logEntry.Timestamp)
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: logEntry.Error}
				}
				cols, rows, affected, err := md.ExecuteQuery(m.ctx, q, m.activeTbl, int64(db.PageSize))
				logEntry.Duration = time.Since(logEntry.Timestamp)
				logEntry.Operation = "MONGO"
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				logEntry.RowsAffected = affected
				m.queryLog.Add(logEntry)
				if len(cols) > 0 {
					return QueryResult{Cols: cols, Rows: rows, Affected: affected}
				}
				return QueryResult{Affected: affected}

			case db.KindRedis:
				rd, ok := m.driver.(*db.RedisDriver)
				if !ok {
					logEntry.Error = fmt.Errorf("redis driver mismatch")
					logEntry.Duration = time.Since(logEntry.Timestamp)
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: logEntry.Error}
				}
				cols, rows, affected, err := rd.ExecuteQuery(m.ctx, q)
				logEntry.Duration = time.Since(logEntry.Timestamp)
				logEntry.Operation = "REDIS"
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				logEntry.RowsAffected = affected
				m.queryLog.Add(logEntry)
				if len(cols) > 0 {
					return QueryResult{Cols: cols, Rows: rows, Affected: affected}
				}
				return QueryResult{Affected: affected}

			case db.KindCassandra:
				cd, ok := m.driver.(*db.CassandraDriver)
				if !ok {
					logEntry.Error = fmt.Errorf("cassandra driver mismatch")
					logEntry.Duration = time.Since(logEntry.Timestamp)
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: logEntry.Error}
				}
				cols, rows, affected, err := cd.ExecuteQuery(m.ctx, q)
				logEntry.Duration = time.Since(logEntry.Timestamp)
				logEntry.Operation = "CASSANDRA"
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				logEntry.RowsAffected = affected
				m.queryLog.Add(logEntry)
				if len(cols) > 0 {
					return QueryResult{Cols: cols, Rows: rows, Affected: affected}
				}
				return QueryResult{Affected: affected}
			}
		}

		if isDestructiveQuery(q) {
			res, execErr := m.driver.Exec(m.ctx, q)
			logEntry.Duration = time.Since(logEntry.Timestamp)
			if execErr != nil {
				logEntry.Error = execErr
				m.queryLog.Add(logEntry)
				return ErrMsg{Err: execErr}
			}
			n, _ := res.RowsAffected()
			logEntry.Operation = "EXEC"
			logEntry.RowsAffected = n
			m.queryLog.Add(logEntry)
			return QueryResult{Affected: n}
		}

		rows, err := m.driver.Query(m.ctx, q)
		if err == nil {
			defer func() { _ = rows.Close() }()
			cols, _ := rows.Columns()
			if len(cols) > 0 {
				data, scanErrs, _ := db.ScanRows(rows, len(cols))
				logEntry.Duration = time.Since(logEntry.Timestamp)
				logEntry.RowsAffected = int64(len(data))
				m.queryLog.Add(logEntry)
				return QueryResult{Cols: cols, Rows: data, Affected: int64(len(data)), ScanErrs: scanErrs}
			}
		}
		res, execErr := m.driver.Exec(m.ctx, q)
		logEntry.Duration = time.Since(logEntry.Timestamp)
		if execErr != nil {
			logEntry.Error = execErr
			m.queryLog.Add(logEntry)
			return ErrMsg{Err: execErr}
		}
		n, _ := res.RowsAffected()
		logEntry.Operation = "EXEC"
		logEntry.RowsAffected = n
		m.queryLog.Add(logEntry)
		return QueryResult{Affected: n}
	}
}

// --- Export ---

func (m Model) execExport(fmtType string) tea.Cmd {
	return func() tea.Msg {
		cols := m.colNames(m.activeTbl)
		var total int
		rows, err := m.driver.Query(m.ctx, fmt.Sprintf("SELECT COUNT(*) FROM %q", m.activeTbl))
		if err == nil {
			rows.Next()
			_ = rows.Scan(&total)
			_ = rows.Close()
		}

		base := strings.TrimSuffix(filepath.Base(m.dbPath), filepath.Ext(m.dbPath))
		fname := base + "_" + filepath.Base(m.activeTbl)
		fname = strings.ReplaceAll(fname, "..", "")
		info := m.schema[m.activeTbl]

		switch fmtType {
		case "csv":
			return m.exportCSV(fname, cols, total)
		case "json":
			return m.exportJSON(fname, cols, total)
		case "xlsx":
			return m.exportXLSX(fname, cols, total)
		case "sql":
			return m.exportSQL(base, fname, cols, info, total)
		}
		return ErrMsg{Err: fmt.Errorf("unknown format")}
	}
}

func (m Model) exportCSV(fname string, cols []string, total int) tea.Msg {
	path := fname + ".csv"
	f, err := os.Create(path)
	if err != nil {
		return ErrMsg{Err: err}
	}
	defer func() { _ = f.Close() }()
	bw := bufio.NewWriter(f)
	w := csv.NewWriter(bw)
	_ = w.Write(cols)
	exported := 0
	scanErrs := 0
	for offset := 0; offset < total; offset += db.PageSize {
		q := fmt.Sprintf("SELECT %s FROM %q LIMIT %d OFFSET %d", strings.Join(cols, ", "), m.activeTbl, db.PageSize, offset)
		rows, err := m.driver.Query(m.ctx, q)
		if err != nil {
			return ErrMsg{Err: err}
		}
		data, se, _ := db.ScanRows(rows, len(cols))
		_ = rows.Close()
		scanErrs += se
		for _, row := range data {
			_ = w.Write(row)
			exported++
		}
	}
	w.Flush()
	_ = bw.Flush()
	msg := fmt.Sprintf("Exported %d rows to %s", exported, path)
	if scanErrs > 0 {
		msg += fmt.Sprintf(" (%d scan errors)", scanErrs)
	}
	return NotifyMsg{Msg: msg}
}

func (m Model) exportJSON(fname string, cols []string, total int) tea.Msg {
	path := fname + ".json"
	f, err := os.Create(path)
	if err != nil {
		return ErrMsg{Err: err}
	}
	defer func() { _ = f.Close() }()
	bw := bufio.NewWriter(f)
	_, _ = bw.WriteString("[")
	exported := 0
	scanErrs := 0
	first := true
	for offset := 0; offset < total; offset += db.PageSize {
		q := fmt.Sprintf("SELECT %s FROM %q LIMIT %d OFFSET %d", strings.Join(cols, ", "), m.activeTbl, db.PageSize, offset)
		rows, err := m.driver.Query(m.ctx, q)
		if err != nil {
			return ErrMsg{Err: err}
		}
		data, se, _ := db.ScanRows(rows, len(cols))
		_ = rows.Close()
		scanErrs += se
		for _, row := range data {
			if !first {
				_, _ = bw.WriteString(",\n")
			}
			first = false
			rec := make(map[string]string)
			for i, col := range cols {
				if i < len(row) {
					rec[col] = row[i]
				}
			}
			b, _ := json.Marshal(rec)
			_, _ = bw.Write(b)
			exported++
		}
	}
	_, _ = bw.WriteString("\n]")
	_ = bw.Flush()
	msg := fmt.Sprintf("Exported %d rows to %s", exported, path)
	if scanErrs > 0 {
		msg += fmt.Sprintf(" (%d scan errors)", scanErrs)
	}
	return NotifyMsg{Msg: msg}
}

func (m Model) exportXLSX(fname string, cols []string, total int) tea.Msg {
	path := fname + ".xlsx"
	f := excelize.NewFile()
	sheet := "Sheet1"
	for i, col := range cols {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, col)
	}
	exported := 0
	scanErrs := 0
	ri := 2
	for offset := 0; offset < total; offset += db.PageSize {
		q := fmt.Sprintf("SELECT %s FROM %q LIMIT %d OFFSET %d", strings.Join(cols, ", "), m.activeTbl, db.PageSize, offset)
		rows, err := m.driver.Query(m.ctx, q)
		if err != nil {
			return ErrMsg{Err: err}
		}
		data, se, _ := db.ScanRows(rows, len(cols))
		_ = rows.Close()
		scanErrs += se
		for _, row := range data {
			for ci, val := range row {
				cell, _ := excelize.CoordinatesToCellName(ci+1, ri)
				_ = f.SetCellValue(sheet, cell, val)
			}
			ri++
			exported++
		}
	}
	for i := range cols {
		cn, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, cn, cn, 20)
	}
	if err := f.SaveAs(path); err != nil {
		return ErrMsg{Err: err}
	}
	msg := fmt.Sprintf("Exported %d rows to %s", exported, path)
	if scanErrs > 0 {
		msg += fmt.Sprintf(" (%d scan errors)", scanErrs)
	}
	return NotifyMsg{Msg: msg}
}

func (m Model) exportSQL(base, fname string, cols []string, info []db.ColInfo, total int) tea.Msg {
	path := fname + ".sql"
	f, err := os.Create(path)
	if err != nil {
		return ErrMsg{Err: err}
	}
	defer func() { _ = f.Close() }()
	bw := bufio.NewWriter(f)
	_, _ = fmt.Fprintf(bw, "-- SQLite dump: %s.%s\n\n", base, m.activeTbl)
	sr, _ := m.driver.Query(m.ctx, fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name=%q", m.activeTbl))
	if sr != nil {
		for sr.Next() {
			var s string
			if sr.Scan(&s) == nil {
				_, _ = bw.WriteString(s + ";\n\n")
			}
		}
		_ = sr.Close()
	}
	exported := 0
	for offset := 0; offset < total; offset += db.PageSize {
		q := fmt.Sprintf("SELECT %s FROM %q LIMIT %d OFFSET %d", strings.Join(cols, ", "), m.activeTbl, db.PageSize, offset)
		rows, err := m.driver.Query(m.ctx, q)
		if err != nil {
			return ErrMsg{Err: err}
		}
		data, _, _ := db.ScanRows(rows, len(cols))
		_ = rows.Close()
		for _, row := range data {
			var cs, vs []string
			for i, cell := range row {
				if i < len(cols) {
					cs = append(cs, fmt.Sprintf("%q", cols[i]))
					isNum := false
					if i < len(info) {
						t := strings.ToUpper(info[i].Type)
						isNum = strings.Contains(t, "INT") || strings.Contains(t, "REAL") || strings.Contains(t, "FLOA") || strings.Contains(t, "NUM") || strings.Contains(t, "DOUB")
					}
					if isNum {
						if _, e := strconv.ParseFloat(cell, 64); e == nil {
							vs = append(vs, cell)
						} else {
							vs = append(vs, fmt.Sprintf("'%s'", strings.ReplaceAll(cell, "'", "''")))
						}
					} else {
						vs = append(vs, fmt.Sprintf("'%s'", strings.ReplaceAll(cell, "'", "''")))
					}
				}
			}
			_, _ = fmt.Fprintf(bw, "INSERT INTO %q (%s) VALUES (%s);\n", m.activeTbl, strings.Join(cs, ", "), strings.Join(vs, ", "))
			exported++
		}
	}
	_ = bw.Flush()
	return NotifyMsg{Msg: fmt.Sprintf("Exported %d rows to %s", exported, path)}
}

// --- Import ---

// safeImportPath validates that an import path does not escape the current
// working directory and resolves to an existing regular file.
func safeImportPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path must not contain '..': %s", path)
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", path)
	}
	return absPath, nil
}

func (m Model) execImport(path, format string) tea.Cmd {
	return func() tea.Msg {
		safePath, err := safeImportPath(path)
		if err != nil {
			return ErrMsg{Err: err}
		}
		f, err := os.Open(safePath)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("open file: %w", err)}
		}
		defer func() { _ = f.Close() }()

		var rows [][]string
		switch format {
		case "csv":
			r := csv.NewReader(f)
			_, err := r.Read()
			if err != nil {
				return ErrMsg{Err: fmt.Errorf("read csv header: %w", err)}
			}
			for {
				record, err := r.Read()
				if err != nil {
					break
				}
				rows = append(rows, record)
			}
		case "json":
			var data []map[string]interface{}
			dec := json.NewDecoder(f)
			if err := dec.Decode(&data); err != nil {
				return ErrMsg{Err: err}
			}
			cols := m.colNames(m.activeTbl)
			for _, rec := range data {
				var row []string
				for _, col := range cols {
					val := ""
					if v, ok := rec[col]; ok {
						val = fmt.Sprintf("%v", v)
					}
					row = append(row, val)
				}
				rows = append(rows, row)
			}
		}

		inserted := 0
		failed := 0
		cols := m.colNames(m.activeTbl)
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		q := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)", m.activeTbl, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
		for _, row := range rows {
			args := make([]interface{}, len(cols))
			for i := range cols {
				if i < len(row) {
					args[i] = row[i]
				} else {
					args[i] = nil
				}
			}
			if _, err := m.driver.Exec(m.ctx, q, args...); err != nil {
				failed++
			} else {
				inserted++
			}
		}
		msg := fmt.Sprintf("Imported %d rows from %s", inserted, filepath.Base(path))
		if failed > 0 {
			msg += fmt.Sprintf(" (%d failed)", failed)
		}
		return NotifyMsg{Msg: msg}
	}
}
