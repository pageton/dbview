package app

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"dbview/internal/db"
	"dbview/internal/table"
	"dbview/internal/theme"
)

func keyText(msg tea.KeyMsg) string {
	if len(msg.Runes) == 0 {
		return ""
	}
	return string(msg.Runes)
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

func trimLastWord(s string) string {
	r := []rune(s)
	end := len(r)
	for end > 0 && unicode.IsSpace(r[end-1]) {
		end--
	}
	for end > 0 && !unicode.IsSpace(r[end-1]) {
		end--
	}
	return string(r[:end])
}

func runeLen(s string) int {
	return len([]rune(s))
}

func clampCursor(cursor, n int) int {
	if cursor < 0 {
		return 0
	}
	if cursor > n {
		return n
	}
	return cursor
}

func insertAtRunePos(s string, pos int, text string) string {
	r := []rune(s)
	p := clampCursor(pos, len(r))
	ins := []rune(text)
	out := make([]rune, 0, len(r)+len(ins))
	out = append(out, r[:p]...)
	out = append(out, ins...)
	out = append(out, r[p:]...)
	return string(out)
}

func deleteRuneBeforePos(s string, pos int) (string, int) {
	r := []rune(s)
	p := clampCursor(pos, len(r))
	if p == 0 {
		return s, 0
	}
	out := append(r[:p-1], r[p:]...)
	return string(out), p - 1
}

func deleteWordBeforePos(s string, pos int) (string, int) {
	r := []rune(s)
	p := clampCursor(pos, len(r))
	if p == 0 {
		return s, 0
	}
	start := p
	for start > 0 && unicode.IsSpace(r[start-1]) {
		start--
	}
	for start > 0 && !unicode.IsSpace(r[start-1]) {
		start--
	}
	out := append(r[:start], r[p:]...)
	return string(out), start
}

func deleteRuneAtPos(s string, pos int) (string, int) {
	r := []rune(s)
	p := clampCursor(pos, len(r))
	if p >= len(r) {
		return s, p
	}
	out := append(r[:p], r[p+1:]...)
	return string(out), p
}

func wordBackward(s string, pos int) int {
	r := []rune(s)
	p := clampCursor(pos, len(r))
	if p == 0 {
		return 0
	}
	// Skip trailing whitespace
	for p > 0 && unicode.IsSpace(r[p-1]) {
		p--
	}
	// Skip word characters
	for p > 0 && !unicode.IsSpace(r[p-1]) {
		p--
	}
	return p
}

func wordForward(s string, pos int) int {
	r := []rune(s)
	n := len(r)
	p := clampCursor(pos, n)
	if p >= n {
		return n
	}
	// Skip current word characters
	for p < n && !unicode.IsSpace(r[p]) {
		p++
	}
	// Skip following whitespace
	for p < n && unicode.IsSpace(r[p]) {
		p++
	}
	return p
}

func parseCommaInput(input string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(input))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	fields, err := r.Read()
	if err != nil {
		return nil, err
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields, nil
}

// Update handles the top-level Bubble Tea update dispatch.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.view == ViewData || m.view == ViewQuery || m.view == ViewSearch {
			m = m.refreshTable()
		}
		return m, doTick()

	case TickMsg:
		m.spinner = (m.spinner + 1) % len(theme.SpinnerFrames)
		if m.loading {
			return m, doTick()
		}
		return m, nil

	case NotifyMsg:
		m.loading = false
		m = m.setStatus(msg.Msg)
		return m, doTick()

	case ClipboardMsg:
		if msg.OK {
			m = m.setStatus("Copied to clipboard")
		} else {
			m = m.setStatus(fmt.Sprintf("Copy failed (install xclip, wl-copy, or pbcopy): %s", table.Trunc(msg.Text, 30)))
		}
		return m, doTick()

	case tea.KeyMsg:
		if !m.ready && m.err != nil {
			switch msg.String() {
			case "r", "R":
				nm := New(m.dbPath)
				nm.width = m.width
				nm.height = m.height
				nm.theme = m.theme
				return nm, nil
			case "q", "esc", "ctrl+c":
				if m.cancel != nil {
					m.cancel()
				}
				if m.driver != nil {
					m.driver.Close()
				}
				return m, tea.Quit
			}
		}
		if m.helpVis {
			m.helpVis = false
			return m, nil
		}
		if m.dialog.Kind != DlgNone {
			return m.updateDialog(msg)
		}
		// ctrl+c is the only truly global shortcut
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			if m.driver != nil {
				m.driver.Close()
			}
			return m, tea.Quit
		}
		// In query mode, only text input, enter, esc, backspace, up/down work
		if m.view == ViewQuery {
			return m.updateQuery(msg)
		}
		// T and ? are global only outside query mode
		if msg.String() == "T" {
			for i, t := range theme.ThemeOrder {
				if m.theme == t {
					m.theme = theme.ThemeOrder[(i+1)%len(theme.ThemeOrder)]
					break
				}
			}
			m = m.setStatus("Theme: " + m.theme)
			return m, nil
		}
		if msg.String() == "?" {
			m.helpVis = true
			return m, nil
		}
		if msg.String() == "M" {
			m.mouseOn = !m.mouseOn
			if m.mouseOn {
				m = m.setStatus("Mouse mode enabled")
				return m, tea.EnableMouseCellMotion
			}
			m = m.setStatus("Mouse mode disabled: terminal selection enabled")
			return m, tea.DisableMouse
		}
		// Q opens query log from any view
		if msg.String() == "Q" {
			m.prevView = m.view
			m.view = ViewQueryLog
			m.logCursor = 0
			m.logExpand = false
			return m, nil
		}
		switch m.view {
		case ViewTables:
			return m.updateTables(msg)
		case ViewData:
			return m.updateData(msg)
		case ViewSchema:
			return m.updateSchema(msg)
		case ViewSearch:
			return m.updateSearch(msg)
		case ViewStats:
			m.view = ViewTables
			return m, nil
		case ViewQueryLog:
			return m.updateQueryLog(msg)
		case ViewDetail:
			return m.updateDetail(msg)
		}

	case DataLoaded:
		m.loading = false
		m.err = nil
		m.activeTbl = msg.TblName
		m.dataCols = msg.Cols
		m.allRows = msg.Rows
		m.totalRows = msg.Total
		m.sortCol = -1
		m.sortAsc = true
		m.search = ""
		m.page = 1
		m.colCursor = 0
		m.pages = m.calcPages()
		m.view = ViewData
		m = m.refreshTable()
		statusMsg := ""
		if msg.ScanErrs > 0 {
			statusMsg = fmt.Sprintf(" (%d scan errors)", msg.ScanErrs)
		}
		m = m.setStatus(fmt.Sprintf("Loaded %d rows%s", len(msg.Rows), statusMsg))
		return m, doTick()

	case QueryResult:
		m.err = nil
		if len(msg.Cols) > 0 {
			m.view = ViewData
			m.dataCols = msg.Cols
			m.allRows = msg.Rows
			m.totalRows = len(msg.Rows)
			m.sortCol = -1
			m.activeTbl = "query"
			m.page = 1
			m.colCursor = 0
			m.pages = m.calcPages()
			m.search = ""
			m = m.refreshTable()
			m.affected = int64(len(msg.Rows))
			statusMsg := fmt.Sprintf("Query returned %d row(s)", len(msg.Rows))
			if msg.ScanErrs > 0 {
				statusMsg += fmt.Sprintf(" (%d scan errors)", msg.ScanErrs)
			}
			m = m.setStatus(statusMsg)
		} else {
			m.affected = msg.Affected
			m = m.setStatus(fmt.Sprintf("%d row(s) affected", msg.Affected))
			if m.activeTbl != "" && m.activeTbl != "query" {
				m.refreshCachedStats()
				return m, m.loadData(m.activeTbl)
			}
			return m, doTick()
		}

	case ErrMsg:
		m.loading = false
		m.err = msg.Err
		return m, nil

	case tea.MouseMsg:
		switch m.view {
		case ViewTables:
			return m.updateTablesMouse(msg)
		case ViewData, ViewSearch:
			return m.updateDataMouse(msg)
		}
	}
	return m, nil
}

// --- Dialog handler ---

func (m Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := &m.dialog
	switch d.Kind {
	case DlgConfirm:
		switch msg.String() {
		case "y", "Y":
			if fn, ok := d.Data.(func(Model) (Model, tea.Cmd)); ok {
				nm, cmd := fn(m)
				nm.dialog = Dialog{}
				return nm, cmd
			}
			m.dialog = Dialog{}
		case "n", "N", "esc":
			m.dialog = Dialog{}
		}
	case DlgEdit:
		switch msg.String() {
		case "enter":
			parts, ok := d.Data.([]int)
			if !ok {
				m.dialog = Dialog{}
				return m, nil
			}
			editCol := parts[0]
			cursor := parts[1]
			col := m.dataCols[editCol]
			where, whereArgs := m.pkWhere(cursor)
			if where == "" {
				m.dialog = Dialog{}
				return m.setStatus("Cannot edit: no primary key"), nil
			}
			val := d.Input
			q := fmt.Sprintf("UPDATE %q SET %q = ? WHERE %s", m.activeTbl, col, where)
			args := append([]interface{}{val}, whereArgs...)
			logEntry := db.QueryLogEntry{
				Timestamp: time.Now(),
				Operation: "UPDATE",
				Table:     m.activeTbl,
				Query:     db.FormatSQLUpdate(m.activeTbl, col, val, where),
			}
			m.dialog = Dialog{}
			return m, func() tea.Msg {
				start := time.Now()
				_, err := m.driver.Exec(m.ctx, q, args...)
				logEntry.Duration = time.Since(start)
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				logEntry.RowsAffected = 1
				m.queryLog.Add(logEntry)
				return QueryResult{Affected: 1}
			}
		case "esc":
			m.dialog = Dialog{}
		case "backspace":
			if len(d.Input) > 0 {
				d.Input = trimLastRune(d.Input)
			}
		case "alt+backspace", "ctrl+w":
			if len(d.Input) > 0 {
				d.Input = trimLastWord(d.Input)
			}
		default:
			if text := keyText(msg); text != "" {
				d.Input += text
			}
		}
	case DlgExport:
		switch msg.String() {
		case "1":
			m.dialog = Dialog{}
			m.loading = true
			return m, m.execExport("csv")
		case "2":
			m.dialog = Dialog{}
			m.loading = true
			return m, m.execExport("json")
		case "3":
			m.dialog = Dialog{}
			m.loading = true
			return m, m.execExport("xlsx")
		case "4":
			m.dialog = Dialog{}
			m.loading = true
			return m, m.execExport("sql")
		case "esc":
			m.dialog = Dialog{}
		}
	case DlgAddRow:
		switch msg.String() {
		case "enter":
			if strings.TrimSpace(d.Input) == "" {
				m.dialog = Dialog{}
				return m, nil
			}
			fields, perr := parseCommaInput(d.Input)
			if perr != nil {
				return m.setStatus("Invalid input: use comma-separated values (quote fields with commas)"), nil
			}
			cols := m.colNames(m.activeTbl)

			if m.driver != nil {
				switch m.driver.Kind() {
				case db.KindMongoDB:
					md, ok := m.driver.(*db.MongoDriver)
					if !ok {
						m.dialog = Dialog{}
						return m.setStatus("Internal error: mongo driver mismatch"), nil
					}
					vals := make([]string, len(cols))
					for i := range cols {
						if i < len(fields) {
							vals[i] = fields[i]
						} else {
							vals[i] = "NULL"
						}
					}
					logEntry := db.QueryLogEntry{
						Timestamp: time.Now(),
						Operation: "INSERT",
						Table:     m.activeTbl,
						Query:     fmt.Sprintf("MONGO INSERT %s", m.activeTbl),
					}
					m.dialog = Dialog{}
					return m, func() tea.Msg {
						start := time.Now()
						n, err := md.InsertDocument(m.ctx, m.activeTbl, cols, vals)
						logEntry.Duration = time.Since(start)
						if err != nil {
							logEntry.Error = err
							m.queryLog.Add(logEntry)
							return ErrMsg{Err: err}
						}
						logEntry.RowsAffected = n
						m.queryLog.Add(logEntry)
						return QueryResult{Affected: n}
					}

				case db.KindRedis:
					rd, ok := m.driver.(*db.RedisDriver)
					if !ok {
						m.dialog = Dialog{}
						return m.setStatus("Internal error: redis driver mismatch"), nil
					}
					vals := make([]string, len(cols))
					for i := range cols {
						if i < len(fields) {
							vals[i] = fields[i]
						} else {
							vals[i] = "NULL"
						}
					}
					logEntry := db.QueryLogEntry{
						Timestamp: time.Now(),
						Operation: "INSERT",
						Table:     m.activeTbl,
						Query:     fmt.Sprintf("REDIS INSERT %s", m.activeTbl),
					}
					m.dialog = Dialog{}
					return m, func() tea.Msg {
						start := time.Now()
						n, err := rd.InsertEntry(m.ctx, m.activeTbl, cols, vals)
						logEntry.Duration = time.Since(start)
						if err != nil {
							logEntry.Error = err
							m.queryLog.Add(logEntry)
							return ErrMsg{Err: err}
						}
						logEntry.RowsAffected = n
						m.queryLog.Add(logEntry)
						return QueryResult{Affected: n}
					}
				}
			}

			placeholders := make([]string, len(cols))
			args := make([]interface{}, len(cols))
			valStrs := make([]string, len(cols))
			for i := range cols {
				placeholders[i] = "?"
				if i < len(fields) {
					v := strings.TrimSpace(fields[i])
					valStrs[i] = v
					if v == "NULL" {
						args[i] = nil
					} else {
						args[i] = v
					}
				} else {
					valStrs[i] = "NULL"
					args[i] = nil
				}
			}
			q := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)", m.activeTbl, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
			logEntry := db.QueryLogEntry{
				Timestamp: time.Now(),
				Operation: "INSERT",
				Table:     m.activeTbl,
				Query:     db.FormatSQLInsert(m.activeTbl, cols, valStrs),
			}
			m.dialog = Dialog{}
			return m, func() tea.Msg {
				start := time.Now()
				res, err := m.driver.Exec(m.ctx, q, args...)
				logEntry.Duration = time.Since(start)
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				n, _ := res.RowsAffected()
				logEntry.RowsAffected = n
				m.queryLog.Add(logEntry)
				return QueryResult{Affected: n}
			}
		case "esc":
			m.dialog = Dialog{}
		case "backspace":
			if len(d.Input) > 0 {
				d.Input = trimLastRune(d.Input)
			}
		case "alt+backspace", "ctrl+w":
			if len(d.Input) > 0 {
				d.Input = trimLastWord(d.Input)
			}
		default:
			if text := keyText(msg); text != "" {
				d.Input += text
			}
		}
	case DlgImportFmt:
		switch msg.String() {
		case "1":
			m.dialog = Dialog{Kind: DlgImportPath, Title: "Import CSV", Body: "Enter file path:", Data: "csv"}
		case "2":
			m.dialog = Dialog{Kind: DlgImportPath, Title: "Import JSON", Body: "Enter file path:", Data: "json"}
		case "esc":
			m.dialog = Dialog{}
		}
	case DlgImportPath:
		switch msg.String() {
		case "enter":
			path := strings.TrimSpace(d.Input)
			if path == "" {
				m.dialog = Dialog{}
				return m, nil
			}
			fmtStr, _ := d.Data.(string)
			m.dialog = Dialog{}
			m.loading = true
			return m, m.execImport(path, fmtStr)
		case "esc":
			m.dialog = Dialog{}
		case "backspace":
			if len(d.Input) > 0 {
				d.Input = trimLastRune(d.Input)
			}
		case "alt+backspace", "ctrl+w":
			if len(d.Input) > 0 {
				d.Input = trimLastWord(d.Input)
			}
		default:
			if text := keyText(msg); text != "" {
				d.Input += text
			}
		}
	case DlgConfirmQuery:
		switch msg.String() {
		case "y", "Y":
			q, _ := d.Data.(string)
			m.dialog = Dialog{}
			if q != "" {
				m.queryHist = append(m.queryHist, q)
				m.qHistIdx = len(m.queryHist)
				return m, m.execQuery(q)
			}
		case "n", "N", "esc":
			m.dialog = Dialog{}
		}
	}
	return m, nil
}

// --- View-specific key handlers ---

func (m Model) updateTables(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.tables)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.tables) > 0 {
			return m, m.loadData(m.tables[m.cursor])
		}
	case "s":
		if len(m.tables) > 0 {
			m.activeTbl = m.tables[m.cursor]
			m.view = ViewSchema
		}
	case "x":
		if len(m.tables) > 0 {
			tbl := m.tables[m.cursor]
			m.dialog = Dialog{
				Kind:  DlgConfirm,
				Title: "DROP TABLE",
				Body:  fmt.Sprintf("Delete table %q?\nThis cannot be undone.", tbl),
				Data: func(m Model) (Model, tea.Cmd) {
					return m, func() tea.Msg {
						start := time.Now()
						_, err := m.driver.Exec(m.ctx, fmt.Sprintf("DROP TABLE %q", tbl))
						dur := time.Since(start)
						logQ := fmt.Sprintf("DROP TABLE %q;", tbl)
						if err != nil {
							m.queryLog.Add(db.QueryLogEntry{
								Timestamp: time.Now(), Operation: "DROP", Table: tbl,
								Query: logQ, Duration: dur, Error: err,
							})
							return ErrMsg{Err: err}
						}
						m.queryLog.Add(db.QueryLogEntry{
							Timestamp: time.Now(), Operation: "DROP", Table: tbl,
							Query: logQ, Duration: dur,
						})
						m.tables = nil
						m.schema = make(map[string][]db.ColInfo)
						m.fks = make(map[string][]db.FKInfo)
						rows, err := m.driver.Query(m.ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
						if err == nil {
							defer rows.Close()
							for rows.Next() {
								var name string
								if rows.Scan(&name) == nil {
									m.tables = append(m.tables, name)
									m.schema[name], _ = m.driver.LoadSchema(m.ctx, name)
									m.fks[name], _ = m.driver.LoadFKs(m.ctx, name)
								}
							}
						}
						m.refreshCachedStats()
						return NotifyMsg{Msg: fmt.Sprintf("Table %q dropped", tbl)}
					}
				},
			}
		}
	case "D":
		m.view = ViewStats
	case "r", "R":
		tables, err := m.driver.ListTables(m.ctx)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.tables = tables
		m.schema = make(map[string][]db.ColInfo)
		m.fks = make(map[string][]db.FKInfo)
		for _, name := range m.tables {
			m.schema[name], _ = m.driver.LoadSchema(m.ctx, name)
			m.fks[name], _ = m.driver.LoadFKs(m.ctx, name)
		}
		if m.cursor >= len(m.tables) {
			m.cursor = len(m.tables) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m = m.setStatus(fmt.Sprintf("Reloaded %d table(s)", len(m.tables)))
	case "F":
		if len(m.tables) > 0 {
			tbl := m.tables[m.cursor]
			rc := m.rowCount(tbl)
			m.dialog = Dialog{
				Kind:  DlgConfirm,
				Title: "FLUSH TABLE",
				Body:  fmt.Sprintf("Delete ALL %d rows from %q?\nThis cannot be undone.", rc, tbl),
				Data: func(m Model) (Model, tea.Cmd) {
					return m, func() tea.Msg {
						start := time.Now()
						_, err := m.driver.Exec(m.ctx, fmt.Sprintf("DELETE FROM %q", tbl))
						dur := time.Since(start)
						logQ := fmt.Sprintf("DELETE FROM %q;", tbl)
						if err != nil {
							m.queryLog.Add(db.QueryLogEntry{
								Timestamp: time.Now(), Operation: "FLUSH", Table: tbl,
								Query: logQ, Duration: dur, Error: err,
							})
							return ErrMsg{Err: err}
						}
						m.queryLog.Add(db.QueryLogEntry{
							Timestamp: time.Now(), Operation: "FLUSH", Table: tbl,
							Query: logQ, Duration: dur,
						})
						return NotifyMsg{Msg: fmt.Sprintf("Table %q flushed (%d rows deleted)", tbl, rc)}
					}
				},
			}
		}
	case "/":
		m.view = ViewQuery
		m.query = ""
		m.queryCursor = 0
		m.err = nil
	}
	return m, nil
}

func (m Model) updateData(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.view = ViewTables
		return m, nil
	case "s":
		m.view = ViewSchema
		return m, nil
	case "/":
		m.view = ViewQuery
		m.query = ""
		m.queryCursor = 0
		m.err = nil
		return m, nil
	case "ctrl+f":
		m.view = ViewSearch
		m.search = ""
		return m, nil
	case "left", "h":
		if m.colCursor > 0 {
			m.colCursor--
		}
		return m, nil
	case "right", "l":
		if m.colCursor < len(m.dataCols)-1 {
			m.colCursor++
		}
		return m, nil
	case "e":
		if m.activeTbl != "" && m.activeTbl != "query" && len(m.dataCols) > 0 {
			cursor := m.dataTbl.Cursor()
			editCol := m.colCursor
			if editCol >= len(m.dataCols) {
				editCol = 0
			}
			colName := m.dataCols[editCol]
			oldVal := ""
			if cursor < len(m.allRows) && editCol < len(m.allRows[cursor]) {
				oldVal = m.allRows[cursor][editCol]
			}
			displayRow := m.globalRowIdx(cursor) + 1
			m.dialog = Dialog{
				Kind:  DlgEdit,
				Title: fmt.Sprintf("Edit %s.%s (row %d)", m.activeTbl, colName, displayRow),
				Body:  fmt.Sprintf("Current: %s\nNew value:", oldVal),
				Input: oldVal,
				Data:  []int{editCol, cursor},
			}
		}
		return m, nil
	case "x":
		if m.activeTbl != "" && m.activeTbl != "query" {
			cursor := m.dataTbl.Cursor()
			where, whereArgs := m.pkWhere(cursor)
			if where == "" {
				return m.setStatus("Cannot delete: no primary key"), nil
			}
			displayRow := m.globalRowIdx(cursor) + 1
			capturedArgs := make([]interface{}, len(whereArgs))
			copy(capturedArgs, whereArgs)
			capturedWhere := where
			tbl := m.activeTbl
			m.dialog = Dialog{
				Kind:  DlgConfirm,
				Title: "DELETE ROW",
				Body:  fmt.Sprintf("Delete row %d?\n%s", displayRow, where),
				Data: func(m Model) (Model, tea.Cmd) {
					return m, func() tea.Msg {
						start := time.Now()
						_, err := m.driver.Exec(m.ctx, fmt.Sprintf("DELETE FROM %q WHERE %s", tbl, capturedWhere), capturedArgs...)
						dur := time.Since(start)
						logQ := db.FormatSQLDelete(tbl, capturedWhere)
						if err != nil {
							m.queryLog.Add(db.QueryLogEntry{
								Timestamp: time.Now(), Operation: "DELETE", Table: tbl,
								Query: logQ, Duration: dur, Error: err,
							})
							return ErrMsg{Err: err}
						}
						m.queryLog.Add(db.QueryLogEntry{
							Timestamp: time.Now(), Operation: "DELETE", Table: tbl,
							Query: logQ, Duration: dur,
						})
						return NotifyMsg{Msg: "Row deleted"}
					}
				},
			}
		}
		return m, nil
	case "d":
		if m.activeTbl != "" && m.activeTbl != "query" {
			if m.driver != nil {
				switch m.driver.Kind() {
				case db.KindMongoDB:
					return m.setStatus("Duplicate row is not supported for MongoDB yet"), nil
				case db.KindRedis:
					return m.setStatus("Duplicate row is not supported for Redis yet"), nil
				}
			}
			cursor := m.dataTbl.Cursor()
			if cursor >= len(m.allRows) {
				return m, nil
			}
			info := m.schema[m.activeTbl]
			var insertCols []string
			var placeholders []string
			var args []interface{}
			for i, c := range info {
				if c.PK && db.IsAutoIncrement(c) {
					continue
				}
				insertCols = append(insertCols, c.Name)
				placeholders = append(placeholders, "?")
				if i < len(m.allRows[cursor]) {
					args = append(args, m.allRows[cursor][i])
				} else {
					args = append(args, nil)
				}
			}
			if len(insertCols) == 0 {
				for i, c := range info {
					insertCols = append(insertCols, c.Name)
					placeholders = append(placeholders, "?")
					if i < len(m.allRows[cursor]) {
						args = append(args, m.allRows[cursor][i])
					} else {
						args = append(args, nil)
					}
				}
			}
			q := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)", m.activeTbl, strings.Join(insertCols, ", "), strings.Join(placeholders, ", "))
			valStrs := make([]string, len(args))
			for i, a := range args {
				if a == nil {
					valStrs[i] = "NULL"
				} else {
					valStrs[i] = fmt.Sprintf("%v", a)
				}
			}
			logEntry := db.QueryLogEntry{
				Timestamp: time.Now(),
				Operation: "INSERT",
				Table:     m.activeTbl,
				Query:     db.FormatSQLInsert(m.activeTbl, insertCols, valStrs),
			}
			return m, func() tea.Msg {
				start := time.Now()
				res, err := m.driver.Exec(m.ctx, q, args...)
				logEntry.Duration = time.Since(start)
				if err != nil {
					logEntry.Error = err
					m.queryLog.Add(logEntry)
					return ErrMsg{Err: err}
				}
				n, _ := res.RowsAffected()
				logEntry.RowsAffected = n
				m.queryLog.Add(logEntry)
				return QueryResult{Affected: n}
			}
		}
		return m, nil
	case "a":
		if m.activeTbl != "" && m.activeTbl != "query" && len(m.dataCols) > 0 {
			ph := make([]string, len(m.dataCols))
			for i := range ph {
				ph[i] = "NULL"
			}
			title := fmt.Sprintf("INSERT INTO %s", m.activeTbl)
			body := fmt.Sprintf("Columns: %s\nValues (comma-sep):", strings.Join(m.dataCols, ", "))
			if m.driver != nil {
				switch m.driver.Kind() {
				case db.KindMongoDB:
					title = fmt.Sprintf("INSERT DOCUMENT %s", m.activeTbl)
					body += "\nTip: values auto-parse (NULL, true/false, numbers, JSON objects/arrays)"
				case db.KindRedis:
					title = fmt.Sprintf("ADD ENTRY %s", m.activeTbl)
					body += "\nTip: key is required; ttl is seconds; use member/score for set/zset"
				}
			}
			m.dialog = Dialog{
				Kind:  DlgAddRow,
				Title: title,
				Body:  body,
				Input: strings.Join(ph, ","),
			}
		}
		return m, nil
	case "I":
		if m.activeTbl != "" && m.activeTbl != "query" {
			m.dialog = Dialog{
				Kind:  DlgImportFmt,
				Title: "IMPORT DATA",
				Body:  fmt.Sprintf("Import into %s from:\n\n  [1] CSV file\n  [2] JSON file", m.activeTbl),
			}
		}
		return m, nil
	case "E":
		m.dialog = Dialog{
			Kind:  DlgExport,
			Title: "EXPORT DATA",
			Body:  fmt.Sprintf("Export %s (all %d rows) as:", m.activeTbl, m.totalRows),
		}
		return m, nil
	case "c":
		cursor := m.dataTbl.Cursor()
		filtered := m.filteredRows()
		sorted := m.sortedRows(filtered)
		if cursor < len(sorted) {
			editCol := m.colCursor
			if editCol < len(m.dataCols) {
				cell := ""
				if editCol < len(sorted[cursor]) {
					cell = sorted[cursor][editCol]
				}
				return m, copyToClipboard(cell)
			}
		}
		return m, nil
	case "C":
		cursor := m.dataTbl.Cursor()
		filtered := m.filteredRows()
		sorted := m.sortedRows(filtered)
		if cursor < len(sorted) {
			return m, copyToClipboard(strings.Join(sorted[cursor], " | "))
		}
		return m, nil
	case "[":
		if m.activeTbl != "query" && m.page > 1 {
			m.page--
			return m, m.loadPage(m.activeTbl, m.page)
		}
		return m, nil
	case "]":
		if m.activeTbl != "query" && m.page < m.pages {
			m.page++
			return m, m.loadPage(m.activeTbl, m.page)
		}
		return m, nil
	case "{":
		if m.activeTbl != "query" {
			m.page = 1
			return m, m.loadPage(m.activeTbl, m.page)
		}
		return m, nil
	case "}":
		if m.activeTbl != "query" {
			m.page = m.pages
			return m, m.loadPage(m.activeTbl, m.page)
		}
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		col := int(msg.String()[0] - '1')
		if col < len(m.dataCols) {
			if col == m.sortCol {
				m.sortAsc = !m.sortAsc
			} else {
				m.sortCol = col
				m.sortAsc = true
			}
			m = m.refreshTable()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.dataTbl, cmd = m.dataTbl.Update(msg)
	return m, cmd
}

func (m Model) updateSchema(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "s":
		m.view = ViewTables
	case "d":
		if m.activeTbl != "" {
			return m, m.loadData(m.activeTbl)
		}
	case "/":
		m.view = ViewQuery
		m.query = ""
		m.err = nil
	}
	return m, nil
}

func (m Model) updateQuery(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(m.dataCols) > 0 {
			m.view = ViewData
		} else {
			m.view = ViewTables
		}
		return m, nil
	case "up":
		if len(m.queryHist) == 0 {
			return m, nil
		}
		if m.qHistIdx > 0 {
			m.qHistIdx--
		}
		if m.qHistIdx >= 0 && m.qHistIdx < len(m.queryHist) {
			m.query = m.queryHist[m.qHistIdx]
			m.queryCursor = runeLen(m.query)
		}
		return m, nil
	case "down":
		if len(m.queryHist) == 0 {
			return m, nil
		}
		if m.qHistIdx < len(m.queryHist)-1 {
			m.qHistIdx++
			m.query = m.queryHist[m.qHistIdx]
			m.queryCursor = runeLen(m.query)
		} else {
			m.qHistIdx = len(m.queryHist)
			m.query = ""
			m.queryCursor = 0
		}
		return m, nil
	case "left":
		if m.queryCursor > 0 {
			m.queryCursor--
		}
		return m, nil
	case "right":
		if m.queryCursor < runeLen(m.query) {
			m.queryCursor++
		}
		return m, nil
	case "home", "ctrl+a":
		m.queryCursor = 0
		return m, nil
	case "end", "ctrl+e":
		m.queryCursor = runeLen(m.query)
		return m, nil
	case "ctrl+left":
		m.queryCursor = wordBackward(m.query, m.queryCursor)
		return m, nil
	case "ctrl+right":
		m.queryCursor = wordForward(m.query, m.queryCursor)
		return m, nil
	case "delete":
		m.query, m.queryCursor = deleteRuneAtPos(m.query, m.queryCursor)
	case "enter":
		q := strings.TrimSpace(m.query)
		if q != "" {
			if isDestructiveQueryForDriver(m.driver, q) {
				m.dialog = Dialog{
					Kind:  DlgConfirmQuery,
					Title: "EXECUTE QUERY",
					Body:  fmt.Sprintf("This query/command may modify data or schema:\n\n  %s\n\nProceed?", q),
					Data:  q,
				}
				return m, nil
			}
			m.queryHist = append(m.queryHist, q)
			m.qHistIdx = len(m.queryHist)
			return m, m.execQuery(q)
		}
	case "backspace":
		if runeLen(m.query) > 0 {
			m.query, m.queryCursor = deleteRuneBeforePos(m.query, m.queryCursor)
		}
	case "alt+backspace", "ctrl+w":
		if runeLen(m.query) > 0 {
			m.query, m.queryCursor = deleteWordBeforePos(m.query, m.queryCursor)
		}
	default:
		if text := keyText(msg); text != "" && text != "/" {
			m.query = insertAtRunePos(m.query, m.queryCursor, text)
			m.queryCursor += runeLen(text)
		}
	}
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.search = ""
		m.view = ViewData
		m = m.refreshTable()
		return m, nil
	case "enter":
		m.view = ViewData
		return m, nil
	case "backspace":
		if len(m.search) > 0 {
			m.search = trimLastRune(m.search)
			m = m.refreshTable()
		}
	case "alt+backspace", "ctrl+w":
		if len(m.search) > 0 {
			m.search = trimLastWord(m.search)
			m = m.refreshTable()
		}
	default:
		if text := keyText(msg); text != "" && text != "/" {
			m.search += text
			m = m.refreshTable()
		}
	}
	return m, nil
}

// --- Query Log handler ---

func (m Model) updateQueryLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := m.queryLog.Len()
	switch msg.String() {
	case "esc", "Q":
		m.view = m.prevView
		m.logExpand = false
		return m, nil
	case "up", "k":
		if m.logCursor > 0 {
			m.logCursor--
		}
		m.logExpand = false
	case "down", "j":
		if m.logCursor < n-1 {
			m.logCursor++
		}
		m.logExpand = false
	case "enter":
		m.logExpand = !m.logExpand
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

// --- Detail view handler ---

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = ViewData
		return m, nil
	case "up", "k":
		if m.logCursor > 0 {
			m.logCursor--
		}
	case "down", "j":
		if m.logCursor < m.totalRows-1 {
			m.logCursor++
		}
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

// --- Mouse handlers ---

func (m Model) updateTablesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.MouseButtonWheelDown:
			if m.cursor < len(m.tables)-1 {
				m.cursor++
			}
		default:
			row := msg.Y - 3
			if row >= 0 && row < len(m.tables) {
				m.cursor = row
				return m, m.loadData(m.tables[m.cursor])
			}
		}
	}
	return m, nil
}

func (m Model) updateDataMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			cur := m.dataTbl.Cursor()
			if cur > 0 {
				m.dataTbl.SetCursor(cur - 1)
			}
		case tea.MouseButtonWheelDown:
			cur := m.dataTbl.Cursor()
			rows := len(m.dataTbl.Rows())
			if cur < rows-1 {
				m.dataTbl.SetCursor(cur + 1)
			}
		default:
			// Forward clicks to the table
			var cmd tea.Cmd
			m.dataTbl, cmd = m.dataTbl.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func isDestructiveQueryForDriver(d db.Driver, q string) bool {
	if d == nil {
		return isDestructiveQuery(q)
	}

	upper := strings.ToUpper(strings.TrimSpace(q))
	if upper == "" {
		return false
	}

	switch d.Kind() {
	case db.KindRedis:
		cmd := upper
		if i := strings.IndexAny(cmd, " \t\n\r"); i >= 0 {
			cmd = cmd[:i]
		}
		writes := map[string]struct{}{
			"SET": {}, "DEL": {}, "EXPIRE": {}, "PEXPIRE": {}, "PERSIST": {},
			"HSET": {}, "HDEL": {}, "RPUSH": {}, "LPUSH": {}, "LSET": {},
			"LREM": {}, "SADD": {}, "SREM": {}, "ZADD": {}, "ZREM": {},
			"FLUSHDB": {}, "FLUSHALL": {},
		}
		_, ok := writes[cmd]
		return ok

	case db.KindMongoDB:
		prefixes := []string{
			"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE",
			"ATTACH", "DETACH", "REPLACE", "CREATE", "UPSERT",
		}
		for _, p := range prefixes {
			if strings.HasPrefix(upper, p) {
				return true
			}
		}
		return false

	default:
		return isDestructiveQuery(q)
	}
}

// isDestructiveQuery returns true if the SQL statement is likely to mutate data
// or schema (INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE, ATTACH, DETACH, REPLACE).
func isDestructiveQuery(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	prefixes := []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE",
		"ATTACH", "DETACH", "REPLACE", "CREATE",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}
