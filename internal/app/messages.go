package app

import (
	"database/sql"
	"time"
)

// ViewMode represents the current UI view.
type ViewMode int

const (
	ViewTables ViewMode = iota
	ViewData
	ViewSchema
	ViewQuery
	ViewSearch
	ViewStats
	ViewQueryLog
	ViewDetail
)

// DialogKind represents the type of dialog shown to the user.
type DialogKind int

const (
	DlgNone DialogKind = iota
	DlgConfirm
	DlgEdit
	DlgExport
	DlgAddRow
	DlgImportFmt
	DlgImportPath
	DlgConfirmQuery
)

// Dialog holds the state for an active dialog overlay.
type Dialog struct {
	Kind   DialogKind
	Title  string
	Body   string
	Input  string
	Cursor int
	Data   interface{}
}

// TickMsg is sent on each timer tick for spinner animation.
type TickMsg time.Time

// ClipboardMsg is sent after a clipboard copy attempt.
type ClipboardMsg struct {
	Text string
	OK   bool
}

// DataLoaded is sent when table data has been loaded from the database.
type DataLoaded struct {
	Cols     []string
	Rows     [][]string
	Total    int
	TblName  string
	Page     int
	ScanErrs int
}

// QueryResult is sent after executing a SQL query.
type QueryResult struct {
	Cols     []string
	Rows     [][]string
	Affected int64
	ScanErrs int
}

// ErrMsg wraps an error for the Bubble Tea update loop.
type ErrMsg struct{ Err error }

// NotifyMsg carries a user-facing notification string.
type NotifyMsg struct{ Msg string }

// ColInfo is re-exported from the db package for use in the model.
type ColInfo = struct {
	CID     int
	Name    string
	Type    string
	NotNull bool
	PK      bool
	Dflt    sql.NullString
}

// FKInfo is re-exported from the db package for use in the model.
type FKInfo = struct {
	ID, Table, From, To string
}
