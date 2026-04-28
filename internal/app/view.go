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

// renderInputWithCursor renders text with a styled cursor at the given rune position.
func renderInputWithCursor(text string, cursor int, _ ...bool) string {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("7")).
		Bold(true)
	if cursor < len(runes) {
		before := string(runes[:cursor])
		ch := string(runes[cursor])
		after := string(runes[cursor+1:])
		return before + cursorStyle.Render(ch) + after
	}
	return string(runes) + cursorStyle.Render(" ")
}

func (m Model) View() string {
	if !m.ready {
		if m.err != nil {
			return m.renderStartupError()
		}
		return m.renderLoading()
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

	if !m.helpVis && m.dialog.Kind == DlgNone {
		content += "\n" + m.renderStatusBar()
	}

	if m.helpVis {
		content += m.renderHelp()
	}
	if m.dialog.Kind != DlgNone {
		content += m.renderDialog()
	}
	return content
}

// --- Header ---

// viewTitle returns the display title for the current view.
func (m Model) viewTitle() string {
	switch m.view {
	case ViewTables:
		return "Tables"
	case ViewData:
		label := m.activeTbl
		if len(m.searches) > 0 {
			quoted := make([]string, len(m.searches))
			for i, s := range m.searches {
				quoted[i] = fmt.Sprintf("%q", s)
			}
			label = fmt.Sprintf("%s [%s] (%d/%d)", m.activeTbl, strings.Join(quoted, "+"), len(m.filteredRowsCache), len(m.allRows))
		}
		return "Data · " + label
	case ViewSchema:
		return "Schema · " + m.activeTbl
	case ViewQuery:
		return "Query"
	case ViewSearch:
		return "Filter · " + m.activeTbl
	case ViewStats:
		return "Stats"
	case ViewQueryLog:
		return "Query Log"
	case ViewDetail:
		return fmt.Sprintf("Detail · %s (%d/%d)", m.activeTbl, m.logCursor+1, len(m.detailRows))
	default:
		return "dbview"
	}
}

// renderHeaderBar creates a full-width header bar with title, provider badge, and status.
func (m Model) renderHeaderBar() string {
	cl := m.c()

	// Left: dbview · ViewTitle
	leftTitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.HeaderFg)).
		Bold(true).
		Render("dbview")
	leftSep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Dim)).
		Render(" · ")
	leftView := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Accent)).
		Bold(true).
		Render(m.viewTitle())

	left := leftTitle + leftSep + leftView

	// Right: provider badge + dsn + status + spinner
	var rightParts []string
	if m.driver != nil {
		badge := lipgloss.NewStyle().
			Background(lipgloss.Color(cl.Accent)).
			Foreground(lipgloss.Color(cl.White)).
			Bold(true).
			Padding(0, 1).
			Render(" " + m.viewerTitle() + " ")
		rightParts = append(rightParts, badge)

		dsn := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Dim)).
			Render(redactDSN(m.dbPath))
		rightParts = append(rightParts, dsn)
	}

	if m.status != "" && time.Now().Before(m.flashEnd) {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Ok)).
			Bold(true)
		rightParts = append(rightParts, statusStyle.Render(m.status))
	}

	if m.loading {
		spinStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Accent)).
			Bold(true)
		rightParts = append(rightParts, spinStyle.Render(theme.SpinnerFrames[m.spinner]))
	}

	right := strings.Join(rightParts, " ")

	bar := lipgloss.NewStyle().
		Background(lipgloss.Color(cl.HeaderBg)).
		Width(m.width).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))

	return lipgloss.NewStyle().Width(m.width).Render(bar)
}

// --- Pagination ---

func (m Model) renderPagination() string {
	if m.activeTbl == "query" || m.pages <= 1 {
		return ""
	}
	cl := m.c()
	from := (m.page-1)*db.PageSize + 1
	to := min(m.page*db.PageSize, m.totalRows)

	info := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Ok)).Render(fmt.Sprintf("Rows %d-%d of %d", from, to, m.totalRows))
	nav := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Background(lipgloss.Color(cl.Accent)).Padding(0, 1).Bold(true).Render(fmt.Sprintf(" %d/%d ", m.page, m.pages))
	navHints := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Dim)).Render("[ ] page · { } jump")

	return lipgloss.NewStyle().Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, " ", info, "  ", nav, " ", navHints))
}

// --- Help overlay ---

func (m Model) renderHelp() string {
	return m.renderHelpNew()
}

// --- Startup ---

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
	case db.KindMariaDB:
		b.WriteString("MariaDB quick checks:\n")
		b.WriteString("- Ensure MariaDB is running and listening on the expected host/port\n")
		b.WriteString("- If using localhost, try 127.0.0.1 to avoid IPv6/::1 issues\n")
		b.WriteString("- Verify database/user credentials and grants\n\n")
	case db.KindCockroachDB:
		b.WriteString("CockroachDB quick checks:\n")
		b.WriteString("- Ensure CockroachDB is running and reachable on the configured host/port\n")
		b.WriteString("- If using localhost, try 127.0.0.1 to avoid IPv6/::1 issues\n")
		b.WriteString("- Verify database/user credentials and access rules\n\n")
	case db.KindMSSQL:
		b.WriteString("MSSQL quick checks:\n")
		b.WriteString("- Ensure SQL Server is running and reachable on the configured host/port\n")
		b.WriteString("- Verify the instance name if using a named instance (sqlserver://host/instance)\n")
		b.WriteString("- Check that TCP/IP connections are enabled in SQL Server Configuration Manager\n\n")
	case db.KindCassandra:
		b.WriteString("Cassandra quick checks:\n")
		b.WriteString("- Ensure Cassandra/ScyllaDB is running and reachable on the configured host/port\n")
		b.WriteString("- Verify the keyspace name in the connection URI (cassandra://host:9042/keyspace)\n")
		b.WriteString("- Check username/password if authentication is enabled\n\n")
	}

	b.WriteString("Press r to retry, q or esc to quit.\n")
	return b.String()
}

func (m Model) renderLoading() string {
	cl := m.c()

	header := lipgloss.NewStyle().
		Background(lipgloss.Color(cl.HeaderBg)).
		Foreground(lipgloss.Color(cl.HeaderFg)).
		Bold(true).
		Width(m.width).
		Padding(0, 1).
		Render("dbview · Connecting...")

	spin := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Accent)).
		Bold(true).
		Render(theme.SpinnerFrames[m.spinner]) + " Initializing..."

	centerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Dim)).
		Align(lipgloss.Center).
		Width(m.width).
		Padding(2, 0)

	return lipgloss.JoinVertical(lipgloss.Center,
		header,
		"\n",
		centerStyle.Render(spin),
	)
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
	case db.KindMariaDB:
		return "MariaDB"
	case db.KindCockroachDB:
		return "CockroachDB"
	case db.KindMSSQL:
		return "MSSQL"
	case db.KindCassandra:
		return "Cassandra"
	default:
		return "dbview"
	}
}

// --- Status Bar ---

func (m Model) renderStatusBar() string {
	cl := m.c()

	var hints []string
	switch m.view {
	case ViewTables:
		hints = []string{"↑↓", "enter", "s schema", "x drop", "F flush", "D stats", "/ sql", "r reload"}
	case ViewData:
		hints = []string{"←→ col", "↑↓ scroll", "enter detail", "1-9 sort", "e edit", "x del", "d dup", "a add", "I import", "E export", "c cell", "C row", "[ ] page", "ctrl+f filter", "r reload", "s schema", "/ sql"}
	case ViewQuery:
		hints = []string{"↑↓ history", "enter exec", "esc back"}
	case ViewSearch:
		hints = []string{"type filter", "enter confirm", "esc clear"}
	default:
		hints = []string{"? help", "q×2 quit"}
	}

	hasHelp := false
	hasQuit := false
	for _, h := range hints {
		if h == "? help" {
			hasHelp = true
		}
		if h == "q×2 quit" || h == "q quit" {
			hasQuit = true
		}
	}
	if !hasHelp {
		hints = append(hints, "? help")
	}
	if !hasQuit {
		hints = append(hints, "q×2 quit")
	}

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Border)).Render(strings.Repeat("─", m.width))
	bar := theme.HelpStyle(cl).Render(" " + strings.Join(hints, " · "))

	return sep + "\n" + bar
}

// --- View renderers ---

func (m Model) renderTables() string {
	cl := m.c()
	var b strings.Builder

	b.WriteString(m.renderHeaderBar())

	if len(m.tables) == 0 {
		var emptyMsg string
		switch m.driver.Kind() {
		case db.KindMongoDB:
			emptyMsg = "No collections found."
		case db.KindRedis:
			emptyMsg = "No keys found."
		case db.KindCassandra:
			emptyMsg = "No tables found."
		default:
			emptyMsg = "No tables found."
		}
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Dim)).
			Align(lipgloss.Center).
			Width(m.width).
			Padding(2, 0)
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render(emptyMsg))
		return b.String()
	}

	// Count label
	countLabel := "tables"
	switch m.driver.Kind() {
	case db.KindMongoDB:
		countLabel = "collections"
	case db.KindRedis:
		countLabel = "keys"
	}
	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Dim)).
		Padding(0, 1)
	b.WriteString("\n")
	b.WriteString(countStyle.Render(fmt.Sprintf(" %d %s", len(m.tables), countLabel)))
	b.WriteString("\n")

	// Calculate name column width for proper alignment
	nameColW := 0
	for _, name := range m.tables {
		if len(name) > nameColW {
			nameColW = len(name)
		}
	}
	maxNameW := m.width - 20
	if maxNameW < 10 {
		maxNameW = 10
	}
	if nameColW > maxNameW {
		nameColW = maxNameW
	}

	for i, name := range m.tables {
		rc := m.rowCount(name)

		if i == m.cursor {
			// Selected row highlight
			bg := cl.Accent
			nameCell := lipgloss.NewStyle().
				Foreground(lipgloss.Color(cl.White)).
				Bold(true).
				Width(nameColW).
				Render(name)
			rowCell := lipgloss.NewStyle().
				Foreground(lipgloss.Color(cl.White)).
				Render(fmt.Sprintf("%d rows", rc))

			rowStyle := lipgloss.NewStyle().
				Background(lipgloss.Color(bg)).
				Foreground(lipgloss.Color(cl.White)).
				Bold(true).
				Padding(0, 1)

			b.WriteString(rowStyle.Render("▶ " + nameCell + "  " + rowCell))
		} else {
			nameCell := lipgloss.NewStyle().
				Foreground(lipgloss.Color(cl.HeaderFg)).
				Width(nameColW).
				Render(name)
			rowCell := lipgloss.NewStyle().
				Foreground(lipgloss.Color(cl.Dim)).
				Render(fmt.Sprintf("%d rows", rc))

			b.WriteString(lipgloss.NewStyle().Padding(0, 2).Render("  " + nameCell + "  " + rowCell))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderData() string {
	cl := m.c()
	var b strings.Builder

	b.WriteString(m.renderHeaderBar())

	// Sort/column info bar
	var infoParts []string
	if m.sortCol >= 0 && m.sortCol < len(m.dataCols) {
		dir := "ASC"
		if !m.sortAsc {
			dir = "DESC"
		}
		infoParts = append(infoParts, fmt.Sprintf("sort: %s %s", m.dataCols[m.sortCol], dir))
	}
	if m.colCursor >= 0 && m.colCursor < len(m.dataCols) {
		infoParts = append(infoParts, fmt.Sprintf("col: %s", m.dataCols[m.colCursor]))
	}
	if len(infoParts) > 0 {
		infoStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Dim)).
			Padding(0, 1)
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("  " + strings.Join(infoParts, " · ")))
	}

	b.WriteString("\n\n")

	bs := theme.BorderedTable(cl)
	b.WriteString(bs.Render(m.dataTbl.View()))

	b.WriteString("\n")
	b.WriteString(m.renderPagination())

	if m.activeTbl != "query" && m.pages > 1 {
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderSchema() string {
	cl := m.c()
	var b strings.Builder

	b.WriteString(m.renderHeaderBar())
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
	b.WriteString(theme.HelpStyle(cl).Render(" esc back · d data · r reload · / sql · ? help · q×2 quit"))
	return b.String()
}

func (m Model) renderStats() string {
	cl := m.c()
	var b strings.Builder
	b.WriteString(m.renderHeaderBar())
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
				_ = szRows.Scan(&sz)
			}
			_ = szRows.Close()
		}
		if sz == "" {
			sz = "—"
		}
		printRow([]string{tbl, fmt.Sprintf("%d", rc), sz})
	}
	b.WriteString(theme.HelpStyle(cl).Render(" esc back · ? help · q×2 quit"))
	return b.String()
}

func (m Model) renderQuery() string {
	cl := m.c()
	var b strings.Builder

	b.WriteString(m.renderHeaderBar())
	b.WriteString("\n")

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
		case db.KindCassandra:
			title = "CQL Query"
			prompt = " Enter CQL query (SELECT, INSERT, UPDATE, DELETE, TABLES)"
		}
	}
	_ = title + ctxLabel // consumed by header bar via viewTitle

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
		bs := theme.BorderedTable(cl)
		b.WriteString(bs.Render(m.dataTbl.View()))
		b.WriteString("\n")
		b.WriteString(theme.OkStyle(cl).Render(fmt.Sprintf(" %d row(s)", len(m.allRows))))
		b.WriteString("\n")
	}
	b.WriteString(theme.HelpStyle(cl).Render(" enter execute · ↑↓ history · esc back · ? help · q×2 quit"))
	return b.String()
}

func (m Model) renderSearch() string {
	cl := m.c()
	var b strings.Builder

	b.WriteString(m.renderHeaderBar())
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
	b.WriteString(theme.DimStyle(cl).Render(fmt.Sprintf(" %d/%d rows match", len(m.filteredRowsCache), len(m.allRows))))
	b.WriteString("\n")
	bs := theme.BorderedTable(cl)
	b.WriteString(bs.Render(m.dataTbl.View()))
	b.WriteString("\n")
	helps := " enter add filter · ↑↓ history · esc clear all · ctrl+d remove last · ? help · q×2 quit"
	b.WriteString(theme.HelpStyle(cl).Render(helps))
	return b.String()
}

// --- Query Log View ---

func (m Model) renderQueryLog() string {
	cl := m.c()
	var b strings.Builder
	b.WriteString(m.renderHeaderBar())
	b.WriteString("\n")
	entries := m.queryLog.Entries
	if len(entries) == 0 {
		b.WriteString(theme.DimStyle(cl).Render(" No queries logged yet.\n Execute some operations (edit, insert, delete, etc.) to see them here."))
		b.WriteString("\n")
	} else {
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			isSelected := (len(entries) - 1 - i) == m.logCursor
			prefix := "  "
			style := lipgloss.NewStyle().Width(m.width - 4)
			if isSelected {
				prefix = "> "
				bg := cl.Accent
				style = lipgloss.NewStyle().
					Background(lipgloss.Color(bg)).
					Foreground(lipgloss.Color(cl.White)).
					Bold(true).
					Width(m.width - 4)
			}
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
	b.WriteString(theme.HelpStyle(cl).Render(" ↑↓ navigate · enter expand · esc back · ? help · q×2 quit"))
	return b.String()
}

// --- Detail View ---

func (m Model) renderDetail() string {
	cl := m.c()

	// --- Header ---
	header := m.renderHeaderBar()

	if m.logCursor < 0 || m.logCursor >= len(m.detailRows) {
		emptyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color(cl.Dim)).
			Align(lipgloss.Center).
			Width(m.width).
			Height(m.height - 4).
			Render("No data for this row.")
		return lipgloss.JoinVertical(lipgloss.Top, header, emptyMsg)
	}

	row := m.detailRows[m.logCursor]

	// --- Build field list ---
	maxNameW := 0
	for _, c := range m.dataCols {
		if len(c) > maxNameW {
			maxNameW = len(c)
		}
	}

	var lines []string
	for i, col := range m.dataCols {
		val := ""
		if i < len(row) {
			val = row[i]
		}
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.Accent)).Bold(true).Width(maxNameW + 2)
		valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White))
		lines = append(lines, nameStyle.Render(col+" :")+valStyle.Render(" "+val))
	}

	contentBlock := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))

	// --- Navigation hint ---
	posLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.Dim)).
		Width(m.width).
		Align(lipgloss.Center).
		Render(fmt.Sprintf("Row %d/%d", m.logCursor+1, len(m.detailRows)))

	helpBar := theme.HelpStyle(cl).Render(" ↑↓ navigate rows · esc back · c copy row · ? help · q×2 quit")

	// --- Assemble with vertical centering ---
	// Available height for content: total height minus header(1) minus pos(1) minus help(1) minus margins(2)
	contentH := m.height - 5
	if contentH < 3 {
		contentH = 3
	}
	padTop := (contentH - len(lines)) / 2
	if padTop < 0 {
		padTop = 0
	}
	padBlock := lipgloss.NewStyle().Height(padTop).Width(m.width).Render("")

	return lipgloss.JoinVertical(lipgloss.Top,
		header,
		padBlock,
		posLabel,
		"",
		contentBlock,
		"",
		helpBar,
	)
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
				"", theme.DimStyle(cl).Render("enter confirm · esc cancel"),
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
				"", theme.DimStyle(cl).Render("enter confirm · esc cancel"),
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
				"", theme.DimStyle(cl).Render("enter import · esc cancel"),
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
