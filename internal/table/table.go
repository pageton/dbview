package table

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pageton/dbview/internal/theme"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Trunc truncates s to max runes, appending "..." if truncated.
func Trunc(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// TruncMiddle truncates s to max runes by keeping the start and end,
// inserting "…" in the middle. Useful for column headers where the
// suffix often contains the most distinguishing part (e.g., _id, _hash).
func TruncMiddle(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max <= 5 {
		return Trunc(s, max)
	}
	runes := []rune(s)
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
		w := max(avail-4, minW)
		return []table.Column{{Title: cols[0], Width: w}}
	}
	totalMin := minW * n
	if totalMin > avail {
		w := max(avail/n, 3)
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
		shrinkable := 0
		for _, w := range widths {
			if w > minW {
				shrinkable++
			}
		}
		if shrinkable > 0 {
			base := over / shrinkable
			extra := over % shrinkable
			for i := range widths {
				if widths[i] > minW {
					take := base
					if extra > 0 {
						take++
						extra--
					}
					if take > widths[i]-minW {
						take = widths[i] - minW
					}
					widths[i] -= take
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

// FilteredRows filters rows by multiple case-insensitive search terms.
// All terms must match (AND logic) for a row to be included.
func FilteredRows(allRows [][]string, searches []string) [][]string {
	if len(searches) == 0 {
		return allRows
	}
	terms := make([]string, len(searches))
	for i, s := range searches {
		terms[i] = strings.ToLower(s)
	}
	var out [][]string
	for _, row := range allRows {
		lowered := make([]string, len(row))
		for i, cell := range row {
			lowered[i] = strings.ToLower(cell)
		}
		match := true
		for _, term := range terms {
			found := false
			for _, lc := range lowered {
				if strings.Contains(lc, term) {
					found = true
					break
				}
			}
			if !found {
				match = false
				break
			}
		}
		if match {
			out = append(out, row)
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
	loweredCol := make([]string, len(sorted))
	for i, row := range sorted {
		if sortCol < len(row) {
			loweredCol[i] = strings.ToLower(row[sortCol])
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		cmp := strings.Compare(loweredCol[i], loweredCol[j])
		if sortAsc {
			return cmp < 0
		}
		return cmp > 0
	})
	return sorted
}

// BuildTable creates a table.Model from rows, columns, dimensions, sort state, and theme.
func BuildTable(rows [][]string, cols []string, width, height, sortCol int, sortAsc bool, cl theme.Colors) table.Model {
	avail := max(width-6, 20)
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
	h := max(height-12, 3)
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
