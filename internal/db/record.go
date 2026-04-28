package db

import (
	"fmt"
	"strings"
	"time"
)

// ValueKind identifies the type of a value for display.
type ValueKind string

const (
	ValueNull   ValueKind = "null"
	ValueString ValueKind = "string"
	ValueNumber ValueKind = "number"
	ValueBool   ValueKind = "bool"
	ValueJSON   ValueKind = "json"
	ValueBinary ValueKind = "binary"
)

// StructuredValue holds a formatted value in multiple representations.
type StructuredValue struct {
	Raw     interface{} // original value
	Display string      // formatted for TUI cell (truncated)
	Detail  string      // expanded view (full detail)
	Kind    ValueKind
}

// FormatStructuredValue creates a StructuredValue from a raw interface{} value.
func FormatStructuredValue(v interface{}) StructuredValue {
	var display, detail string
	var kind ValueKind

	if v == nil {
		return StructuredValue{Raw: nil, Display: "NULL", Detail: "NULL", Kind: ValueNull}
	}

	switch val := v.(type) {
	case []byte:
		if isPrintable(val) {
			s := string(val)
			trimmed := strings.TrimSpace(s)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				kind = ValueJSON
				detail = formatJSON(s)
				display = truncateForCell(detail)
			} else {
				kind = ValueString
				detail = s
				display = truncateForCell(s)
			}
		} else {
			kind = ValueBinary
			detail = hexDump(val)
			display = truncateForCell(detail)
		}
	case string:
		s := val
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			kind = ValueJSON
			detail = formatJSON(s)
			display = truncateForCell(detail)
		} else {
			kind = ValueString
			detail = s
			display = truncateForCell(s)
		}
	default:
		s := fmt.Sprintf("%v", v)
		kind = ValueString
		detail = s
		display = truncateForCell(s)
	}

	return StructuredValue{Raw: v, Display: display, Detail: detail, Kind: kind}
}

func truncateForCell(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:197] + "..."
}

// RecordView represents a single row/document/key in expanded detail form.
type RecordView struct {
	TableKind  TableKind
	Fields     []FieldDefinition
	Values     []StructuredValue
	DocumentID interface{} // MongoDB
	Key        string      // Redis
	TTL        time.Duration
	RedisType  string
}

// NewRecordView creates a RecordView from field names and raw values.
func NewRecordView(kind TableKind, fields []FieldDefinition, values []interface{}) RecordView {
	svs := make([]StructuredValue, len(values))
	for i := range values {
		svs[i] = FormatStructuredValue(values[i])
	}
	return RecordView{TableKind: kind, Fields: fields, Values: svs}
}

// QueryLogEntry represents a single logged query/mutation.
type QueryLogEntry struct {
	Timestamp    time.Time
	Operation    string // "INSERT", "UPDATE", "DELETE", "SELECT", "DROP"
	Table        string // table/collection/key name
	Query        string // the actual query/command executed
	NativeQuery  string // database-native representation
	RowsAffected int64
	Duration     time.Duration
	Error        error
}

// QueryLog is a ring buffer of query log entries.
type QueryLog struct {
	Entries []QueryLogEntry
	MaxSize int
}

// NewQueryLog creates a new QueryLog with default max size 100.
func NewQueryLog() QueryLog {
	return QueryLog{
		Entries: make([]QueryLogEntry, 0, 100),
		MaxSize: 100,
	}
}

// Add appends an entry to the log, evicting oldest if at capacity.
func (l *QueryLog) Add(entry QueryLogEntry) {
	l.Entries = append(l.Entries, entry)
	if len(l.Entries) > l.MaxSize {
		copy(l.Entries, l.Entries[len(l.Entries)-l.MaxSize:])
		l.Entries = l.Entries[:l.MaxSize]
	}
}

// Len returns the number of entries.
func (l QueryLog) Len() int {
	return len(l.Entries)
}

// FormatMongoQuery formats a MongoDB operation in readable display.
func FormatMongoQuery(collection, operation string, query string) string {
	return fmt.Sprintf("db.%s.%s(%s)", collection, operation, query)
}

// FormatRedisCommand formats a Redis command in readable display.
func FormatRedisCommand(key, operation string, args ...string) string {
	if len(args) == 0 {
		return fmt.Sprintf("%s %s", operation, key)
	}
	return fmt.Sprintf("%s %s %s", operation, key, strings.Join(args, " "))
}

// FormatSQLInsert formats an INSERT statement with column names and values.
func FormatSQLInsert(table string, cols []string, vals []string) string {
	quotedVals := make([]string, 0, len(vals))
	for i := range vals {
		if i < len(cols) {
			quotedVals = append(quotedVals, fmt.Sprintf("'%s'", strings.ReplaceAll(vals[i], "'", "''")))
		}
	}
	if len(quotedVals) == 0 {
		return fmt.Sprintf("INSERT INTO %s (...) VALUES (...);", table)
	}
	return fmt.Sprintf("INSERT INTO %s (%s)\nVALUES (%s);", table, strings.Join(cols, ", "), strings.Join(quotedVals, ", "))
}

// FormatSQLUpdate formats an UPDATE statement.
func FormatSQLUpdate(table, col, val, where string) string {
	return fmt.Sprintf("UPDATE %s\nSET %s = '%s'\nWHERE %s;", table, col, strings.ReplaceAll(val, "'", "''"), where)
}

// FormatSQLDelete formats a DELETE statement.
func FormatSQLDelete(table, where string) string {
	return fmt.Sprintf("DELETE FROM %s\nWHERE %s;", table, where)
}
