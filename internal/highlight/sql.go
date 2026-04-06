package highlight

import (
	"strings"
	"unicode"

	"dbview/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// tokenType classifies a segment of query text for coloring.
type tokenType int

const (
	tokIdent    tokenType = iota // identifiers, table/column names
	tokKeyword                   // SQL / driver keywords
	tokString                    // single-quoted strings
	tokNumber                    // numeric literals
	tokOperator                  // punctuation and operators
	tokComment                   // -- line comments
)

// token represents a classified segment of the input.
type token struct {
	typ  tokenType
	text string
}

// sqlKeywords is the set of SQL keywords highlighted across all SQL drivers.
var sqlKeywords = map[string]struct{}{
	"SELECT": {}, "FROM": {}, "WHERE": {}, "INSERT": {}, "INTO": {},
	"UPDATE": {}, "DELETE": {}, "DROP": {}, "ALTER": {}, "CREATE": {},
	"TABLE": {}, "INDEX": {}, "AND": {}, "OR": {}, "NOT": {}, "IN": {},
	"LIKE": {}, "JOIN": {}, "ON": {}, "AS": {}, "ORDER": {}, "BY": {},
	"GROUP": {}, "HAVING": {}, "LIMIT": {}, "OFFSET": {}, "SET": {},
	"VALUES": {}, "NULL": {}, "IS": {}, "DISTINCT": {}, "COUNT": {},
	"SUM": {}, "AVG": {}, "MIN": {}, "MAX": {}, "EXISTS": {}, "BETWEEN": {},
	"CASE": {}, "WHEN": {}, "THEN": {}, "ELSE": {}, "END": {}, "UNION": {},
	"ALL": {}, "PRIMARY": {}, "KEY": {}, "FOREIGN": {}, "REFERENCES": {},
	"CASCADE": {}, "INNER": {}, "LEFT": {}, "RIGHT": {}, "OUTER": {},
	"FULL": {}, "CROSS": {}, "NATURAL": {}, "ASC": {}, "DESC": {},
	"IF": {}, "BEGIN": {}, "COMMIT": {}, "ROLLBACK": {}, "TRANSACTION": {},
	"ATTACH": {}, "DETACH": {}, "REPLACE": {}, "TRUNCATE": {}, "WITH": {},
	"EXPLAIN": {}, "PRAGMA": {}, "VACUUM": {}, "ANALYZE": {},
	"RECURSIVE": {}, "VIEW": {}, "TRIGGER": {}, "TEMP": {}, "TEMPORARY": {},
	"UNIQUE": {}, "CHECK": {}, "DEFAULT": {}, "CONSTRAINT": {},
	"ADD": {}, "COLUMN": {}, "RENAME": {}, "TO": {}, "AFTER": {},
	"INTEGER": {}, "TEXT": {}, "REAL": {}, "BLOB": {}, "BOOLEAN": {},
	"VARCHAR": {}, "CHAR": {}, "DATETIME": {}, "DATE": {}, "TIME": {},
	"TIMESTAMP": {}, "FLOAT": {}, "DOUBLE": {}, "DECIMAL": {}, "NUMERIC": {},
	"INT": {}, "BIGINT": {}, "SMALLINT": {}, "TINYINT": {},
	"TRUE": {}, "FALSE": {},
	"COALESCE": {}, "IFNULL": {}, "NULLIF": {}, "CAST": {},
	"RETURNING": {}, "CONFLICT": {}, "IGNORE": {},
	"OVER": {}, "PARTITION": {}, "ROW": {}, "ROWS": {}, "RANGE": {},
	"PRECEDING": {}, "FOLLOWING": {}, "CURRENT": {}, "UNBOUNDED": {},
	"TOP": {}, "FETCH": {}, "NEXT": {}, "ONLY": {}, "FIRST": {},
	"ABSOLUTE": {}, "RELATIVE": {}, "SCROLL": {}, "NO": {}, "CURSOR": {},
	"FOR": {}, "GRANT": {}, "REVOKE": {}, "PRIVILEGES": {},
	"USING": {},
}

// mongoKeywords covers MongoDB shell commands and operators.
var mongoKeywords = map[string]struct{}{
	"find": {}, "count": {}, "collections": {}, "insert": {},
	"update": {}, "delete": {}, "aggregate": {}, "sort": {},
	"limit": {}, "skip": {}, "pretty": {}, "findOne": {},
	"insertOne": {}, "insertMany": {}, "updateOne": {}, "updateMany": {},
	"deleteOne": {}, "deleteMany": {}, "replaceOne": {},
	"createIndex": {}, "dropIndex": {}, "getIndexes": {},
	"distinct": {}, "group": {},
}

// mongoOperators covers MongoDB field-level operators (with $ prefix).
var mongoOperators = map[string]struct{}{
	"$match": {}, "$group": {}, "$sort": {}, "$limit": {}, "$skip": {},
	"$unwind": {}, "$lookup": {}, "$project": {}, "$addFields": {},
	"$count": {}, "$facet": {}, "$bucket": {}, "$bucketAuto": {},
	"$out": {}, "$merge": {}, "$replaceRoot": {}, "$set": {},
	"$unset": {}, "$cond": {}, "$switch": {}, "$ifNull": {},
	"$eq": {}, "$ne": {}, "$gt": {}, "$gte": {}, "$lt": {}, "$lte": {},
	"$in": {}, "$nin": {}, "$and": {}, "$or": {}, "$not": {},
	"$exists": {}, "$type": {}, "$regex": {}, "$expr": {},
	"$sum": {}, "$avg": {}, "$min": {}, "$max": {}, "$first": {},
	"$last": {}, "$push": {}, "$addToSet": {},
}

// redisCommands covers common Redis commands.
var redisCommands = map[string]struct{}{
	"GET": {}, "SET": {}, "DEL": {}, "KEYS": {}, "HGETALL": {},
	"LRANGE": {}, "SMEMBERS": {}, "ZRANGE": {}, "EXPIRE": {},
	"TTL": {}, "EXISTS": {}, "INCR": {}, "DECR": {}, "LPUSH": {},
	"RPUSH": {}, "SADD": {}, "ZADD": {}, "HSET": {}, "HDEL": {},
	"FLUSHDB": {}, "FLUSHALL": {}, "MGET": {}, "MSET": {},
	"APPEND": {}, "STRLEN": {}, "GETSET": {}, "INCRBY": {},
	"DECRBY": {}, "INCRBYFLOAT": {}, "GETRANGE": {}, "SETRANGE": {},
	"HGET": {}, "HMGET": {}, "HMSET": {}, "HKEYS": {}, "HVALS": {},
	"HEXISTS": {}, "HINCRBY": {}, "HLEN": {},
	"LPOP": {}, "RPOP": {}, "LLEN": {}, "LINDEX": {}, "LSET": {},
	"LREM": {}, "LTRIM": {}, "RPOPLPUSH": {},
	"SCARD": {}, "SISMEMBER": {}, "SREM": {}, "SDIFF": {},
	"SINTER": {}, "SUNION": {}, "SMOVE": {}, "SPOP": {},
	"ZCARD": {}, "ZSCORE": {}, "ZREM": {}, "ZRANK": {},
	"ZREVRANK": {}, "ZREVRANGE": {}, "ZRANGEBYSCORE": {},
	"ZREMANGEBYRANK": {}, "ZREMANGEBYSCORE": {}, "ZCOUNT": {},
	"SCAN": {}, "HSCAN": {}, "SSCAN": {}, "ZSCAN": {},
	"INFO": {}, "DBSIZE": {}, "TYPE": {}, "RENAME": {},
	"PERSIST": {}, "PEXPIRE": {}, "PTTL": {},
	"PING": {}, "ECHO": {}, "SELECT": {}, "MOVE": {},
	"OBJECT": {}, "WAIT": {}, "DUMP": {}, "RESTORE": {},
	"SORT": {}, "BITCOUNT": {}, "BITOP": {},
	"PFADD": {}, "PFCOUNT": {}, "PFMERGE": {},
	"GEOADD": {}, "GEODIST": {}, "GEORADIUS": {},
	"SUBSCRIBE": {}, "PUBLISH": {}, "UNSUBSCRIBE": {},
	"MULTI": {}, "EXEC": {}, "DISCARD": {}, "WATCH": {},
	"SCRIPT": {}, "EVAL": {}, "EVALSHA": {},
	"CLUSTER": {}, "READONLY": {}, "READWRITE": {},
	"AUTH": {}, "CLIENT": {}, "CONFIG": {}, "COMMAND": {},
	"SLOWLOG": {}, "MEMORY": {}, "LATENCY": {},
	"XADD": {}, "XLEN": {}, "XRANGE": {}, "XREAD": {},
}

// tokenize splits the input string into classified tokens.
// The driverKind parameter selects the keyword set.
func tokenize(input string, driverKind string) []token {
	var tokens []token
	runes := []rune(input)
	n := len(runes)
	pos := 0

	kwSet := sqlKeywords
	isMongo := driverKind == "mongodb"
	isRedis := driverKind == "redis"
	if isMongo {
		kwSet = nil // handled separately
	}
	if isRedis {
		kwSet = nil // handled separately
	}

	for pos < n {
		r := runes[pos]

		// --- Whitespace ---
		if unicode.IsSpace(r) {
			start := pos
			for pos < n && unicode.IsSpace(runes[pos]) {
				pos++
			}
			tokens = append(tokens, token{tokIdent, string(runes[start:pos])})
			continue
		}

		// --- Single-line comment ---
		if pos+1 < n && runes[pos] == '-' && runes[pos+1] == '-' {
			start := pos
			for pos < n && runes[pos] != '\n' {
				pos++
			}
			tokens = append(tokens, token{tokComment, string(runes[start:pos])})
			continue
		}

		// --- Single-quoted string ---
		if r == '\'' {
			start := pos
			pos++
			for pos < n {
				if runes[pos] == '\'' {
					pos++
					// Doubled quote is an escape inside the string
					if pos < n && runes[pos] == '\'' {
						pos++
						continue
					}
					break
				}
				pos++
			}
			tokens = append(tokens, token{tokString, string(runes[start:pos])})
			continue
		}

		// --- Double-quoted identifier ---
		if r == '"' {
			start := pos
			pos++
			for pos < n {
				if runes[pos] == '"' {
					pos++
					if pos < n && runes[pos] == '"' {
						pos++
						continue
					}
					break
				}
				pos++
			}
			tokens = append(tokens, token{tokIdent, string(runes[start:pos])})
			continue
		}

		// --- Number ---
		if unicode.IsDigit(r) || (r == '.' && pos+1 < n && unicode.IsDigit(runes[pos+1])) {
			start := pos
			hasDot := false
			for pos < n {
				if unicode.IsDigit(runes[pos]) {
					pos++
				} else if runes[pos] == '.' && !hasDot {
					hasDot = true
					pos++
				} else {
					break
				}
			}
			tokens = append(tokens, token{tokNumber, string(runes[start:pos])})
			continue
		}

		// --- Operators and punctuation ---
		if isOperatorRune(r) {
			start := pos
			pos++
			// Check for two-character operators
			if pos < n {
				two := string(runes[start : pos+1])
				if two == "<>" || two == "!=" || two == "<=" || two == ">=" || two == "||" {
					pos++
				}
			}
			tokens = append(tokens, token{tokOperator, string(runes[start:pos])})
			continue
		}

		// --- MongoDB $operator ---
		if isMongo && r == '$' {
			start := pos
			pos++
			for pos < n && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos])) {
				pos++
			}
			text := string(runes[start:pos])
			if _, ok := mongoOperators[strings.ToUpper(text)]; ok {
				tokens = append(tokens, token{tokKeyword, text})
			} else {
				tokens = append(tokens, token{tokIdent, text})
			}
			continue
		}

		// --- Word (identifier or keyword) ---
		if unicode.IsLetter(r) || r == '_' {
			start := pos
			for pos < n && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_') {
				pos++
			}
			text := string(runes[start:pos])
			upper := strings.ToUpper(text)

			if isMongo {
				if _, ok := mongoKeywords[text]; ok {
					tokens = append(tokens, token{tokKeyword, text})
				} else if _, ok := mongoOperators[upper]; ok {
					tokens = append(tokens, token{tokKeyword, text})
				} else {
					tokens = append(tokens, token{tokIdent, text})
				}
			} else if isRedis {
				if _, ok := redisCommands[upper]; ok {
					tokens = append(tokens, token{tokKeyword, text})
				} else {
					tokens = append(tokens, token{tokIdent, text})
				}
			} else {
				if _, ok := kwSet[upper]; ok {
					tokens = append(tokens, token{tokKeyword, text})
				} else {
					tokens = append(tokens, token{tokIdent, text})
				}
			}
			continue
		}

		// --- Fallback: single character as identifier ---
		tokens = append(tokens, token{tokIdent, string(r)})
		pos++
	}

	return tokens
}

// isOperatorRune returns true for characters that form operators/punctuation.
func isOperatorRune(r rune) bool {
	switch r {
	case '=', '<', '>', '!', '+', '-', '*', '/', '%', '|',
		'(', ')', ',', ';', '.', '{', '}', '[', ']', ':':
		return true
	}
	return false
}

// colorForToken returns the theme color string for a given token type.
func colorForToken(typ tokenType, cl theme.Colors) string {
	switch typ {
	case tokKeyword:
		return cl.Accent
	case tokString:
		return cl.Ok
	case tokNumber:
		return cl.Warn
	case tokOperator:
		return cl.Dim
	case tokComment:
		return cl.Dim
	default:
		return cl.White
	}
}

// Highlight tokenizes the query, applies syntax-aware coloring, and inserts a
// styled cursor at the given rune position. The driverKind selects the keyword
// set ("sqlite", "mysql", "postgresql", "mongodb", "redis").
func Highlight(query string, cursor int, cl theme.Colors, driverKind string) string {
	tokens := tokenize(query, driverKind)

	// Clamp cursor
	runes := []rune(query)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cl.AccentDim)).
		Background(lipgloss.Color(cl.White)).
		Bold(true)

	var b strings.Builder
	runeOffset := 0

	for _, tok := range tokens {
		tokRunes := []rune(tok.text)
		tokLen := len(tokRunes)
		tokStart := runeOffset
		tokEnd := runeOffset + tokLen

		color := colorForToken(tok.typ, cl)

		if cursor >= tokStart && cursor < tokEnd {
			// Cursor falls inside this token.
			localPos := min(max(cursor-tokStart, 0), tokLen)

			before := string(tokRunes[:localPos])
			ch := string(tokRunes[localPos])
			after := string(tokRunes[localPos+1:])

			if before != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(before))
			}
			b.WriteString(cursorStyle.Render(ch))
			if after != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(after))
			}
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(tok.text))
		}

		runeOffset = tokEnd
	}

	// Cursor at the very end (after all tokens) — render a space with cursor style
	if cursor >= runeOffset {
		b.WriteString(cursorStyle.Render(" "))
	}

	return b.String()
}
