package table

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"dbview/internal/theme"
)

// Trunc truncates s to max runes, appending "..." if truncated.
func Trunc(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// TruncMiddle truncates s to max runes by keeping the start and end,
// inserting "…" in the middle. Useful for column headers where the
// suffix often contains the most distinguishing part (e.g., _id, _hash).
func TruncMiddle(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 5 {
		return Trunc(s, max)
	}
	half := (max - 1) / 2
	return string(runes[:half]) + "…" + string(runes[len(runes)-(max-half-1):])
}

// SortArrow returns " ^" or " v" if the given column is the sort column.
func SortArrow(col, sortCol int, asc bool) string {
	if col != sortCol {
		return ""
	}
	if asc {
		return " ^"
	}
	return " v"
}

// CalcColWidths computes column widths for a table given available width.
func CalcColWidths(cols []string, rows [][]string, avail int) []table.Column {
	n := len(cols)
	if n == 0 {
		return nil
	}
	minW := 10
	if n == 1 {
		w := avail - 4
		if w < minW {
			w = minW
		}
		return []table.Column{{Title: cols[0], Width: w}}
	}
	totalMin := minW * n
	if totalMin > avail {
		w := avail / n
		if w < 3 {
			w = 3
		}
		var out []table.Column
		for _, c := range cols {
			out = append(out, table.Column{Title: TruncMiddle(c, w-1), Width: w})
		}
		return out
	}
	remain := avail - totalMin
	extra := remain / n
	rem := remain - extra*n
	widths := make([]int, n)
	for i := range widths {
		widths[i] = minW + extra
	}
	maxDataLen := make([]int, n)
	for _, row := range rows {
		for i, cell := range row {
			if i < n && len(cell) > maxDataLen[i] {
				maxDataLen[i] = len(cell)
			}
		}
	}
	for i := range widths {
		need := max(len(cols[i])+2, maxDataLen[i]+2)
		if need > widths[i] && need <= 60 {
			widths[i] = need
		}
	}
	widths[0] += rem
	total := 0
	for _, w := range widths {
		total += w
	}
	if total > avail {
		over := total - avail
		for over > 0 {
			for i := range widths {
				if widths[i] > minW && over > 0 {
					widths[i]--
					over--
				}
			}
		}
	}
	var out []table.Column
	for i, c := range cols {
		out = append(out, table.Column{Title: TruncMiddle(c, widths[i]-1), Width: widths[i]})
	}
	return out
}

// FilteredRows filters rows by a case-insensitive search string.
func FilteredRows(allRows [][]string, search string) [][]string {
	if search == "" {
		return allRows
	}
	s := strings.ToLower(search)
	var out [][]string
	for _, row := range allRows {
		for _, cell := range row {
			if strings.Contains(strings.ToLower(cell), s) {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// SortedRows returns a stable sort of rows by the given column index.
func SortedRows(rows [][]string, sortCol int, sortAsc bool, dataColsLen int) [][]string {
	if sortCol < 0 || sortCol >= dataColsLen {
		return rows
	}
	sorted := make([][]string, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if sortCol >= len(a) || sortCol >= len(b) {
			return false
		}
		cmp := strings.Compare(strings.ToLower(a[sortCol]), strings.ToLower(b[sortCol]))
		if sortAsc {
			return cmp < 0
		}
		return cmp > 0
	})
	return sorted
}

// BuildTable creates a table.Model from rows, columns, dimensions, sort state, and theme.
func BuildTable(rows [][]string, cols []string, width, height, sortCol int, sortAsc bool, cl theme.Colors) table.Model {
	avail := width - 6
	if avail < 20 {
		avail = 20
	}
	tblCols := CalcColWidths(cols, rows, avail)
	for i := range tblCols {
		tblCols[i].Title = Trunc(cols[i]+SortArrow(i, sortCol, sortAsc), tblCols[i].Width-1)
	}
	var tblRows []table.Row
	for _, row := range rows {
		r := make(table.Row, len(cols))
		for i, cell := range row {
			if i < len(cols) && i < len(tblCols) {
				r[i] = Trunc(cell, tblCols[i].Width-1)
			}
		}
		tblRows = append(tblRows, r)
	}
	h := height - 12
	if h < 3 {
		h = 3
	}
	t := table.New(
		table.WithColumns(tblCols),
		table.WithRows(tblRows),
		table.WithFocused(true),
		table.WithHeight(h),
	)
	s := table.DefaultStyles()
	s.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White)).Background(lipgloss.Color(cl.Accent)).Bold(true)
	s.Header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cl.White)).Background(lipgloss.Color(cl.AccentDim))
	s.Cell = lipgloss.NewStyle().Foreground(lipgloss.Color(cl.White))
	t.SetStyles(s)
	return t
}
