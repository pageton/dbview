package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// PageSize is the number of rows loaded per page.
const PageSize = 100

// ColInfo describes a single column in a table schema.
type ColInfo struct {
	CID     int
	Name    string
	Type    string
	NotNull bool
	PK      bool
	Dflt    sql.NullString
}

// FKInfo describes a single foreign key relationship.
type FKInfo struct {
	ID, Table, From, To string
}

// IndexInfo describes a database index.
type IndexInfo struct {
	Name    string
	Columns []string
	Unique  bool
}

// LoadSchema loads column metadata for the given SQLite table.
func LoadSchema(db *sql.DB, tbl string) []ColInfo {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(\"%s\")", strings.ReplaceAll(tbl, `"`, `""`)))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cols []ColInfo
	for rows.Next() {
		var c ColInfo
		var nn, pk int
		if rows.Scan(&c.CID, &c.Name, &c.Type, &nn, &c.Dflt, &pk) == nil {
			c.NotNull = nn == 1
			c.PK = pk == 1
			cols = append(cols, c)
		}
	}
	return cols
}

// LoadFKs loads foreign key information for the given SQLite table.
func LoadFKs(db *sql.DB, tbl string) []FKInfo {
	rows, err := db.Query(fmt.Sprintf("PRAGMA foreign_key_list(\"%s\")", strings.ReplaceAll(tbl, `"`, `""`)))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var fks []FKInfo
	for rows.Next() {
		var f FKInfo
		if rows.Scan(&f.ID, &f.Table, &f.From, &f.To) == nil {
			fks = append(fks, f)
		}
	}
	return fks
}

// LoadIndices loads index information for the given SQLite table.
func LoadIndices(db *sql.DB, tbl string) []IndexInfo {
	rows, err := db.Query(fmt.Sprintf("PRAGMA index_list(\"%s\")", strings.ReplaceAll(tbl, `"`, `""`)))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var indices []IndexInfo
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if rows.Scan(&seq, &name, &unique, &origin, &partial) != nil {
			continue
		}
		idx := IndexInfo{Name: name, Unique: unique == 1}
		// Load columns for this index
		cr, err := db.Query(fmt.Sprintf("PRAGMA index_info(\"%s\")", strings.ReplaceAll(name, `"`, `""`)))
		if err == nil {
			for cr.Next() {
				var rank int
				var colName string
				if cr.Scan(&rank, &colName) == nil && colName != "" {
					idx.Columns = append(idx.Columns, colName)
				}
			}
			cr.Close()
		}
		indices = append(indices, idx)
	}
	return indices
}

// ScanRows scans sql.Rows into a slice of string slices.
// Returns the scanned data and the number of scan errors encountered.
func ScanRows(rows *sql.Rows, colCount int) ([][]string, int, error) {
	var data [][]string
	scanErrors := 0
	for rows.Next() {
		vals := make([]interface{}, colCount)
		ptrs := make([]interface{}, colCount)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			scanErrors++
			continue
		}
		row := make([]string, colCount)
		for i, v := range vals {
			row[i] = FormatValue(v)
		}
		data = append(data, row)
	}
	return data, scanErrors, nil
}

// IsAutoIncrement detects if a column is an auto-increment PK.
// Heuristic: PK column with INTEGER type and no default value.
func IsAutoIncrement(c ColInfo) bool {
	if !c.PK {
		return false
	}
	t := strings.ToUpper(c.Type)
	return strings.Contains(t, "INT") && !c.Dflt.Valid
}

// HasPK returns true if the table schema has at least one primary key column.
func HasPK(schema []ColInfo) bool {
	for _, c := range schema {
		if c.PK {
			return true
		}
	}
	return false
}

// ColNames extracts column names from schema info.
func ColNames(schema []ColInfo) []string {
	var names []string
	for _, c := range schema {
		names = append(names, c.Name)
	}
	return names
}

// ColSelectExpr returns the column list for a SELECT, adding rowid
// when the table has no PK.
func ColSelectExpr(schema []ColInfo) string {
	cols := ColNames(schema)
	if !HasPK(schema) {
		return "rowid, " + strings.Join(cols, ", ")
	}
	return strings.Join(cols, ", ")
}

// OpenSQLite opens a SQLite database at the given path and returns the *sql.DB.
func OpenSQLite(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath)
}

// ListTables queries sqlite_master and returns all user table names.
func ListTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}
	return tables, nil
}

// LoadAllSchema loads schema and FK info for all given tables.
func LoadAllSchema(db *sql.DB, tables []string) (map[string][]ColInfo, map[string][]FKInfo) {
	schema := make(map[string][]ColInfo)
	fks := make(map[string][]FKInfo)
	for _, name := range tables {
		schema[name] = LoadSchema(db, name)
		fks[name] = LoadFKs(db, name)
	}
	return schema, fks
}
