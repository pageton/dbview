package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/pageton/dbview/internal/db"
	"github.com/pageton/dbview/internal/highlight"
	"github.com/pageton/dbview/internal/table"
	"github.com/pageton/dbview/internal/theme"
)

// View renders the current UI view.

// renderInputWithCursor renders text with a styled cursor at the given rune position.
func renderInputWithCursor(text string, cursor int, _ ...bool) string {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if cursor < len(runes) {
		before := string(runes[:cursor])
		ch := string(runes[cursor])
		after := string(runes[cursor+1:])
		cursorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("7")).
			Bold(true)
		return before + cursorStyle.Render(ch) + after
	}
	// Cursor at end — show styled space
	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("7")).
		Bold(true)
	return string(runes) + cursorStyle.Render(" ")
}

func (m Model) View() string {
	if !m.ready {
		if m.err != nil {
			return m.renderStartupError()
		}
		return "Loading..."
	}

	var content string
	switch m.view {
	case ViewTables:
		content = m.renderTables()
	case ViewData:
		content = m.renderData()
	case ViewSchema:
		content = m.renderSchema()
	case ViewQuery:
		content = m.renderQuery()
	case ViewSearch:
		content = m.renderSearch()
	case ViewStats:
		content = m.renderStats()
	case ViewQueryLog:
		content = m.renderQueryLog()
	case ViewDetail:
		content = m.renderDetail()
	}
	if m.helpVis {
		content += m.renderHelp()
	}
	if m.dialog.Kind != DlgNone {
		content += m.renderDialog()
	}
	return content
}

func (m Model) renderStartupError() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Error: %v\n\n", m.err)

	kind, _ := db.DetectDriver(m.dbPath)
	switch kind {
	case db.KindSQLite:
		b.WriteString("SQLite quick checks:\n")
		b.WriteString("- Verify the file path exists and is readable\n")
		b.WriteString("- Ensure file permissions allow read/write access\n\n")
	case db.KindMySQL:
		b.WriteString("MySQL quick checks:\n")
		b.WriteString("- Ensure MySQL is running and listening on the expected host/port\n")
		b.WriteString("- If using localhost, try 127.0.0.1 to avoid IPv6/::1 issues\n")
		b.WriteString("- Verify database/user credentials and grants\n\n")
	case db.KindPostgreSQL:
		b.WriteString("PostgreSQL quick checks:\n")
		b.WriteString("- Ensure PostgreSQL is running and reachable on the configured host/port\n")
		b.WriteString("- If using localhost, try 127.0.0.1 to avoid IPv6/::1 issues\n")
		b.WriteString("- Verify database/user credentials and pg_hba.conf access rules\n\n")
	case db.KindMongoDB:
		b.WriteString("MongoDB quick checks:\n")
		b.WriteString("- Ensure MongoDB is running and reachable on the configured host/port\n")
		b.WriteString("- Verify auth database, username/password, and connection URI options\n")
		b.WriteString("- Check TLS/SSL settings if your server requires secure transport\n\n")
	case db.KindRedis:
		b.WriteString("Redis quick checks:\n")
		b.WriteString("- Ensure Redis is running and reachable on the configured host/port\n")
		b.WriteString("- Verify username/password and ACL permissions if auth is enabled\n")
		b.WriteString("- Check TLS settings when using rediss://\n\n")
	}

	b.WriteString("Press r to retry, q or esc to quit.\n")
	return b.String()
}

// --- Header and pagination ---

// renderHelpBar renders a responsive help bar that adapts to screen width.
func (m Model) renderHelpBar(items []string) string {
	cl := m.c()
	full := " " + strings.Join(items, " • ")
	// If it fits, return as-is
	if len(full) <= m.width {
		return theme.HelpStyle(cl).Render(full)
	}
	// Progressive reduction based on width
	if m.width >= 100 {
		// Medium: show essential shortcuts
		medium := []string{"←→ col", "↑↓ scroll", "e edit", "x del", "a add", "E export", "s schema", "/ sql", "? help", "q×2 quit"}
		return theme.HelpStyle(cl).Render(" " + strings.Join(medium, " • "))
	}
	if m.width >= 60 {
		// Compact: minimal set
		compact := []string{"e edit", "x del", "s schema", "? help", "q×2 quit"}
		return theme.HelpStyle(cl).Render(" " + strings.Join(compact, " • "))
	}
	// Tiny: just help and quit
	return theme.HelpStyle(cl).Render(" ? help • q×2 quit")
}

func (m Model) viewerTitle() string {
	if m.driver == nil {
		return "dbview"
	}
	switch m.driver.Kind() {
	case db.KindSQLite:
		return "SQLite"
	case db.KindMySQL:
		return "MySQL"
	case db.KindPostgreSQL:
		return "PostgreSQL"
	case db.KindMongoDB:
		return "MongoDB"
	case db.KindRedis:
		return "Redis"
	default:
		return "dbview"
	}
}

func (m Model) header(title string) string {
	cl := m.c()
	right := ""
	status := ""
	if m.status != "" && time.Now().Before(m.flashEnd) {
		status = "  " + theme.Styled(m.status, cl.Ok)
	}
	spinner := ""
	if m.loading {
		spinner = " " + theme.SpinnerFrames[m.spinner]
	}
	// Calculate title width, ensure right side fits
	titleStyled := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cl.Accent)).Render(title)
	titleLen := len(title) // raw length for measurement
	rightPart := theme.Styled(right, cl.Dim) + spinner + status
	rightRaw := right + spinner
	if m.status != "" && time.Now().Before(m.flashEnd) {
		rightRaw += "  " + m.status
	}
	// Truncate right side if title + right exceeds width
	avail := m.width - titleLen - 2
	if avail < 10 {
		// Truncate title instead
		if m.width > 20 {
			truncLen := m.width - len(rightRaw) - 4
			if truncLen > 0 && truncLen < len(title) {
				title = table.Trunc(title, truncLen)
				titleStyled = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cl.Accent)).Render(title)
			}
		}
	}
	return lipgloss.NewStyle().Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			titleStyled,
			rightPart,
		))
}

func (m Model) renderPagination() string {
	if m.activeTbl == "query" || m.pages <= 1 {
		return ""
	}
	cl := m.c()
	from := (m.page-1)*db.PageSize + 1
	to := min(m.page*db.PageSize, m.totalRows)
	info := fmt.Sprintf("Rows %d-%d of %d", from, to, m.totalRows)
	nav := fmt.Sprintf("Page %d/%d", m.page, m.pages)
	left := " [ ] prev  { } first"
	right := "[ ] next  { } last"
	return lipgloss.NewStyle().Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			theme.Styled(left, cl.Dim),
			theme.Styled(" "+info+" ", cl.Ok),
			lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Background(lipgloss.Color(cl.Accent)).Padding(0, 2).Bold(true).Render(nav),
			theme.Styled(right, cl.Dim),
		))
}

// --- Help overlay ---

func (m Model) renderHelp() string {
	cl := m.c()
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(cl.Accent)).
		Padding(1, 3).
		Width(m.width - 6).
		Background(lipgloss.Color(cl.Bg))

	var b strings.Builder
	b.WriteString(box.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			theme.StyledBold(fmt.Sprintf("%s Viewer — Help", m.viewerTitle()), cl.Accent),
			"",
			theme.StyledBold("TABLES VIEW", cl.Accent),
			theme.Styled("  ↑↓/jk     Navigate tables", cl.White),
			theme.Styled("  enter     Open table data", cl.White),
			theme.Styled("  s        View schema", cl.White),
			theme.Styled("  x        Drop table (confirm)", cl.White),
			theme.Styled("  D        Database stats", cl.White),
			theme.Styled("  F        Flush table (with confirmation)", cl.White),
			theme.Styled("  /        SQL query", cl.White),
			"",
			theme.StyledBold("DATA VIEW", cl.Accent),
			theme.Styled("  ←→/hl    Select column", cl.White),
			theme.Styled("  ↑↓       Scroll rows", cl.White),
			theme.Styled("  1-9      Sort by column N", cl.White),
			theme.Styled("  e        Edit cell (confirm)", cl.White),
			theme.Styled("  x        Delete row (confirm)", cl.White),
			theme.Styled("  d        Duplicate row", cl.White),
			theme.Styled("  a        Add row", cl.White),
			theme.Styled("  I        Import CSV/JSON", cl.White),
			theme.Styled("  E        Export (CSV/JSON/XLSX/SQL)", cl.White),
			theme.Styled("  c        Copy cell to clipboard", cl.White),
			theme.Styled("  C        Copy row to clipboard", cl.White),
			theme.Styled("  [ ]      Previous/next page", cl.White),
			theme.Styled("  { }      First/last page", cl.White),
			theme.Styled("  ctrl+f    Live filter", cl.White),
			theme.Styled("  s        View schema", cl.White),
			theme.Styled("  /        SQL query", cl.White),
			"",
			theme.StyledBold("QUERY VIEW", cl.Accent),
			theme.Styled("  ↑/↓      Query history", cl.White),
			theme.Styled("  enter    Execute query", cl.White),
			theme.Styled("  esc      Back", cl.White),
			"",
			theme.StyledBold("GLOBAL", cl.Accent),
			theme.Styled("  T        Cycle theme (8 themes)", cl.White),
			theme.Styled("  M        Toggle mouse capture / terminal select", cl.White),
			theme.Styled("  ?        This help", cl.White),
			theme.Styled("  q        Quit", cl.White),
			theme.Styled("  esc      Go back / cancel", cl.White),
			theme.Styled("  ctrl+c    Force quit", cl.White),
		)))
	return b.String()
}

// --- View renderers ---

func (m Model) renderTables() string {
	var b strings.Builder
	b.WriteString(m.header(fmt.Sprintf("%s Viewer — %s", m.viewerTitle(), redactDSN(m.dbPath))))
	b.WriteString("\n")
	cl := m.c()
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Bold(true).Render(
		fmt.Sprintf(" Tables (%d)", len(m.tables))))
	b.WriteString("\n\n")
	if len(m.tables) == 0 {
		switch m.driver.Kind() {
		case db.KindMongoDB:
			b.WriteString(theme.Styled(" No collections found in this database.", cl.Warn))
			b.WriteString("\n")
			b.WriteString(theme.Styled(" Create/import data first, then press r to reopen connection.", cl.Dim))
			b.WriteString("\n\n")
		case db.KindRedis:
			b.WriteString(theme.Styled(" No keys found in this Redis database.", cl.Warn))
			b.WriteString("\n")
			b.WriteString(theme.Styled(" Add keys first, then press r to reopen connection.", cl.Dim))
			b.WriteString("\n\n")
		default:
			b.WriteString(theme.Styled(" No tables found.", cl.Warn))
			b.WriteString("\n\n")
		}
	}
	for i, name := range m.tables {
		prefix := "  "
		style := lipgloss.NewStyle().Width(m.width - 4)
		if i == m.cursor {
			prefix = "> "
			style = style.Background(lipgloss.Color(cl.Accent)).Foreground(lipgloss.Color(cl.White)).Bold(true).Width(m.width - 4)
		}
		rc := m.rowCount(name)
		b.WriteString(style.Render(fmt.Sprintf("%s%-30s %s", prefix, name, theme.Styled(fmt.Sprintf("(%d rows)", rc), cl.Dim))))
		b.WriteString("\n")
	}
	b.WriteString(theme.HelpStyle(cl).Render(" ↑↓ navigate • enter data • s schema • x drop • F flush • D stats • / sql • r reload • ? help • q×2 quit"))
	return b.String()
}

func (m Model) renderData() string {
	var b strings.Builder
	cl := m.c()
	label := m.activeTbl
	if len(m.searches) > 0 {
		filtered := m.filteredRows()
		quoted := make([]string, len(m.searches))
		for i, s := range m.searches {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		label = fmt.Sprintf("%s [filter: %s] (%d/%d)", m.activeTbl, strings.Join(quoted, " + "), len(filtered), len(m.allRows))
	}
	b.WriteString(m.header(fmt.Sprintf("Data — %s", label)))
	b.WriteString("\n")
	if m.sortCol >= 0 && m.sortCol < len(m.dataCols) {
		dir := "ASC"
		if !m.sortAsc {
			dir = "DESC"
		}
		b.WriteString(theme.Styled(fmt.Sprintf(" Sort: %s %s", m.dataCols[m.sortCol], dir), cl.Dim))
		b.WriteString("\n")
	}
	if m.colCursor >= 0 && m.colCursor < len(m.dataCols) {
		b.WriteString(theme.Styled(fmt.Sprintf(" Col: %s", m.dataCols[m.colCursor]), cl.Warn))
		b.WriteString("\n")
	}
	// Theme-aware border (fixes bug where only "light" theme got colored borders)
	bs := theme.BorderedTable(cl)
	b.WriteString(bs.Render(m.dataTbl.View()))
	b.WriteString("\n")
	b.WriteString(m.renderPagination())
	if m.activeTbl != "query" && m.pages > 1 {
		b.WriteString("\n")
	}
	helps := []string{"←→ col", "↑↓ scroll", "1-9 sort", "e edit", "x del", "d dup", "a add", "I import", "E export", "c cell", "C row", "[ ] page", "ctrl+f filter", "r reload", "s schema", "/ sql", "? help", "q×2 quit"}
	b.WriteString(m.renderHelpBar(helps))
	return b.String()
}

func (m Model) renderSchema() string {
	var b strings.Builder
	cl := m.c()
	b.WriteString(m.header(fmt.Sprintf("Schema — %s", m.activeTbl)))
	b.WriteString("\n")
	info := m.schema[m.activeTbl]
	widths := []int{5, 0, 15, 9, 12, 20}
	totalFixed := 5 + 15 + 9 + 12 + 20 + 20
	nameW := max(m.width-totalFixed, 10)
	widths[1] = nameW
	printRow := func(cols []string) {
		for i, c := range cols {
			w := widths[i]
			if i < len(widths) {
				w = widths[i]
			}
			fmt.Fprintf(&b, " %-*s", w, table.Trunc(c, w))
		}
		b.WriteString("\n")
	}
	printRow([]string{"CID", "Column", "Type", "Not Null", "Primary Key", "Default"})
	b.WriteString(strings.Repeat("-", m.width-2) + "\n")
	for _, c := range info {
		nn, pk, dflt := "—", "—", "—"
		if c.NotNull {
			nn = "YES"
		}
		if c.PK {
			pk = "YES"
		}
		if c.Dflt.Valid {
			dflt = c.Dflt.String
		}
		printRow([]string{fmt.Sprintf("%d", c.CID), c.Name, c.Type, nn, pk, dflt})
	}
	fks := m.fks[m.activeTbl]
	if len(fks) > 0 {
		b.WriteString("\n")
		b.WriteString(theme.StyledBold("Foreign Keys:", cl.Warn))
		b.WriteString("\n")
		printFKRow := func(cols []string) {
			for _, c := range cols {
				fmt.Fprintf(&b, " %s", c)
			}
			b.WriteString("\n")
		}
		printFKRow([]string{"From", "Table", "To"})
		b.WriteString(strings.Repeat("-", m.width-2) + "\n")
		for _, fk := range fks {
			printFKRow([]string{fk.From, fk.Table, fk.To})
		}
	}
	indices, err := m.driver.LoadIndices(m.ctx, m.activeTbl)
	if err == nil && len(indices) > 0 {
		b.WriteString("\n")
		b.WriteString(theme.StyledBold("Indices:", cl.Warn))
		b.WriteString("\n")
		for _, idx := range indices {
			label := idx.Name
			if idx.Unique {
				label += " UNIQUE"
			}
			if len(idx.Columns) > 0 {
				label += " (" + strings.Join(idx.Columns, ", ") + ")"
			}
			b.WriteString("  • " + label + "\n")
		}
	}
	b.WriteString(theme.HelpStyle(cl).Render(" esc back • d data • r reload • / sql • ? help • q×2 quit"))
	return b.String()
}

func (m Model) renderStats() string {
	var b strings.Builder
	cl := m.c()
	b.WriteString(m.header("Database Stats"))
	b.WriteString("\n")
	printRow := func(cols []string) {
		for _, c := range cols {
			fmt.Fprintf(&b, " %s", c)
		}
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cl.Accent)).Render(" Property              Value"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")

	printRow([]string{"Database", redactDSN(m.dbPath)})
	printRow([]string{"Size", m.dbSize()})
	printRow([]string{"Tables", fmt.Sprintf("%d", len(m.tables))})
	totalRows := 0
	for _, tbl := range m.tables {
		totalRows += m.rowCount(tbl)
	}
	printRow([]string{"Total Rows", fmt.Sprintf("%d", totalRows)})
	printRow([]string{"Theme", m.theme})

	b.WriteString("\n")
	b.WriteString(theme.StyledBold("Tables:", cl.Accent))
	b.WriteString("\n")
	printRow([]string{"Table", "Rows", "Size"})
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")
	for _, tbl := range m.tables {
		rc := m.rowCount(tbl)
		sz := "—"
		szRows, szErr := m.driver.Query(m.ctx, fmt.Sprintf("SELECT SUM(pgsize) FROM dbstat WHERE name=%q", tbl))
		if szErr == nil {
			if szRows.Next() {
				szRows.Scan(&sz)
			}
			szRows.Close()
		}
		if sz == "" {
			sz = "—"
		}
		printRow([]string{tbl, fmt.Sprintf("%d", rc), sz})
	}
	b.WriteString(theme.HelpStyle(cl).Render(" esc back • ? help • q×2 quit"))
	return b.String()
}

func (m Model) renderQuery() string {
	var b strings.Builder
	cl := m.c()
	title := "SQL Query"
	prompt := " Enter SQL and press Enter to execute | Up/Down: history"
	ctxLabel := ""
	if strings.TrimSpace(m.activeTbl) != "" {
		ctxLabel = fmt.Sprintf(" — %s", m.activeTbl)
	}
	if m.driver != nil {
		switch m.driver.Kind() {
		case db.KindMongoDB:
			title = "Mongo Query"
			prompt = " Use: collections | find <collection> [json] | count <collection> [json] | {json}"
		case db.KindRedis:
			title = "Redis Command"
			prompt = " Enter Redis command (GET/SET/DEL/KEYS/HGETALL/LRANGE/SMEMBERS/ZRANGE)"
		}
	}
	b.WriteString(m.header(title + ctxLabel))
	b.WriteString("\n")
	b.WriteString(theme.WarnStyle(cl).Render(prompt))
	b.WriteString("\n")
	driverKind := ""
	if m.driver != nil {
		driverKind = string(m.driver.Kind())
	}
	highlighted := highlight.Highlight(m.query, m.queryCursor, cl, driverKind)
	fmt.Fprintf(&b, " > %s", highlighted)
	b.WriteString("\n")
	if m.err != nil {
		b.WriteString(theme.ErrStyle(cl).Render(fmt.Sprintf(" %v", m.err)))
		b.WriteString("\n")
	}
	if m.affected > 0 && len(m.dataCols) == 0 {
		b.WriteString(theme.OkStyle(cl).Render(fmt.Sprintf(" %d row(s) affected", m.affected)))
		b.WriteString("\n")
	}
	if len(m.dataCols) > 0 {
		// Theme-aware border (fixes bug where only "light" theme got colored borders)
		bs := theme.BorderedTable(cl)
		b.WriteString(bs.Render(m.dataTbl.View()))
		b.WriteString("\n")
		b.WriteString(theme.OkStyle(cl).Render(fmt.Sprintf(" %d row(s)", len(m.allRows))))
		b.WriteString("\n")
	}
	b.WriteString(theme.HelpStyle(cl).Render(" enter execute • ↑↓ history • esc back • ? help • q×2 quit"))
	return b.String()
}

func (m Model) renderSearch() string {
	var b strings.Builder
	cl := m.c()
	b.WriteString(m.header(fmt.Sprintf("Filter — %s", m.activeTbl)))
	b.WriteString("\n")
	b.WriteString(theme.WarnStyle(cl).Render(" Type to filter rows. Use + to separate terms. Enter to confirm."))
	b.WriteString("\n")
	// Show active filter tags
	if len(m.searches) > 0 {
		var tags []string
		for _, s := range m.searches {
			tags = append(tags, lipgloss.NewStyle().
				Foreground(lipgloss.Color(cl.White)).
				Background(lipgloss.Color(cl.Accent)).
				Padding(0, 1).
				Render(s))
		}
		b.WriteString(" Active: ")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tags...))
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Render(fmt.Sprintf(" /%s", renderInputWithCursor(m.searchInput, m.searchCursor))))
	b.WriteString("\n")
	filtered := m.filteredRows()
	b.WriteString(theme.DimStyle(cl).Render(fmt.Sprintf(" %d/%d rows match", len(filtered), len(m.allRows))))
	b.WriteString("\n")
	// Theme-aware border (fixes bug where only "light" theme got colored borders)
	bs := theme.BorderedTable(cl)
	b.WriteString(bs.Render(m.dataTbl.View()))
	b.WriteString("\n")
	helps := " enter add filter • ↑↓ history • esc clear all • ctrl+d remove last • ? help • q×2 quit"
	b.WriteString(theme.HelpStyle(cl).Render(helps))
	return b.String()
}

// --- Query Log View ---

func (m Model) renderQueryLog() string {
	var b strings.Builder
	cl := m.c()
	b.WriteString(m.header("Query Log"))
	b.WriteString("\n")
	entries := m.queryLog.Entries
	if len(entries) == 0 {
		b.WriteString(theme.DimStyle(cl).Render(" No queries logged yet.\n Execute some operations (edit, insert, delete, etc.) to see them here."))
		b.WriteString("\n")
	} else {
		// Show entries in reverse chronological order
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			isSelected := (len(entries) - 1 - i) == m.logCursor
			prefix := "  "
			style := lipgloss.NewStyle().Width(m.width - 4)
			if isSelected {
				prefix = "> "
				style = style.Background(lipgloss.Color(cl.Accent)).Foreground(lipgloss.Color(cl.White)).Bold(true).Width(m.width - 4)
			}
			// Operation badge
			opColor := cl.Ok
			switch e.Operation {
			case "DELETE", "DROP":
				opColor = cl.Err
			case "INSERT":
				opColor = cl.Ok
			case "UPDATE":
				opColor = cl.Warn
			}
			opBadge := lipgloss.NewStyle().Foreground(lipgloss.Color(opColor)).Bold(true).Render(fmt.Sprintf("[%s]", e.Operation))
			ts := e.Timestamp.Format("15:04:05")
			dur := ""
			if e.Duration > 0 {
				dur = fmt.Sprintf(" (%s)", e.Duration.Round(time.Microsecond))
			}
			errMark := ""
			if e.Error != nil {
				errMark = " FAILED"
			}
			line := fmt.Sprintf("%s %s %s%s  %s", prefix, ts, opBadge, errMark, dur)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
			// Show query preview (first line)
			if isSelected && m.logExpand {
				queryLines := strings.SplitSeq(e.Query, "\n")
				for ql := range queryLines {
					b.WriteString(style.Render("    " + ql))
					b.WriteString("\n")
				}
				if e.Error != nil {
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Err)).Render("    Error: " + e.Error.Error()))
					b.WriteString("\n")
				}
				if e.RowsAffected > 0 {
					b.WriteString(style.Render(fmt.Sprintf("    Rows affected: %d", e.RowsAffected)))
					b.WriteString("\n")
				}
			} else {
				// Show truncated query preview
				preview := e.Query
				if idx := strings.Index(preview, "\n"); idx >= 0 {
					preview = preview[:idx]
				}
				if len(preview) > 80 {
					preview = preview[:77] + "..."
				}
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Dim)).Render("    " + preview))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString(theme.HelpStyle(cl).Render(" ↑↓ navigate • enter expand • esc back • ? help • q×2 quit"))
	return b.String()
}

// --- Detail View ---

func (m Model) renderDetail() string {
	var b strings.Builder
	cl := m.c()
	b.WriteString(m.header(fmt.Sprintf("Detail — %s (row %d/%d)", m.activeTbl, m.logCursor+1, m.totalRows)))
	b.WriteString("\n")
	// Show current row as key-value pairs
	if m.logCursor >= 0 && m.logCursor < len(m.allRows) {
		row := m.allRows[m.logCursor]
		maxNameW := 0
		for _, c := range m.dataCols {
			if len(c) > maxNameW {
				maxNameW = len(c)
			}
		}
		for i, col := range m.dataCols {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Accent)).Bold(true).Width(maxNameW + 2)
			valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White))
			b.WriteString(nameStyle.Render(col + " :"))
			b.WriteString(valStyle.Render(" " + val))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(theme.DimStyle(cl).Render(" No data for this row."))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(theme.HelpStyle(cl).Render(" ↑↓ navigate rows • esc back • ? help • q×2 quit"))
	return b.String()
}

// --- Dialog renderer ---

func (m Model) renderDialog() string {
	d := m.dialog
	cl := m.c()
	switch d.Kind {
	case DlgConfirm:
		box := theme.BoxError(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.ErrStyle(cl).Bold(true).Render("!! "+d.Title),
				"", d.Body, "",
				theme.OkStyle(cl).Render("[Y] Yes")+"   "+theme.DimStyle(cl).Render("[N] No / Esc"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgEdit:
		box := theme.BoxAccent(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.WarnStyle(cl).Bold(true).Render(">> "+d.Title),
				"", d.Body, "",
				lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Render("> "+renderInputWithCursor(d.Input, d.Cursor)),
				"", theme.DimStyle(cl).Render("enter confirm • esc cancel"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgExport:
		box := theme.BoxOk(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.OkStyle(cl).Bold(true).Render(">> "+d.Title),
				"", d.Body, "",
				"  [1] CSV (.csv)",
				"  [2] JSON (.json)",
				"  [3] Excel (.xlsx)",
				"  [4] SQL Dump (.sql)",
				"", theme.DimStyle(cl).Render("esc cancel"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgAddRow:
		box := theme.BoxAccent(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.WarnStyle(cl).Bold(true).Render("++ "+d.Title),
				"", d.Body, "",
				lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Render("> "+renderInputWithCursor(d.Input, d.Cursor)),
				"", theme.DimStyle(cl).Render("enter confirm • esc cancel"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgImportFmt:
		box := theme.BoxAccent(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.WarnStyle(cl).Bold(true).Render(">> "+d.Title),
				"", d.Body, "",
				theme.DimStyle(cl).Render("esc cancel"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgImportPath:
		box := theme.BoxAccent(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.WarnStyle(cl).Bold(true).Render(">> "+d.Title),
				"", d.Body, "",
				lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Render("> "+renderInputWithCursor(d.Input, d.Cursor)),
				"", theme.DimStyle(cl).Render("enter import • esc cancel"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	case DlgConfirmQuery:
		box := theme.BoxError(cl).Width(m.width - 8).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				theme.ErrStyle(cl).Bold(true).Render("!! "+d.Title),
				"", d.Body, "",
				theme.OkStyle(cl).Render("[Y] Yes, execute")+"   "+theme.DimStyle(cl).Render("[N] No / Esc"),
			),
		)
		return lipgloss.NewStyle().MarginTop(1).Render(box)
	}
	return ""
}
