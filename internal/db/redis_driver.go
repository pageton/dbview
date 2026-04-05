package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDriver implements Driver for Redis key-value stores.
// It maps Redis keys into a virtual table structure for the TUI.
type RedisDriver struct {
	client *redis.Client
}

func (d *RedisDriver) Open(ctx context.Context, dsn string) error {
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		return fmt.Errorf("parse redis URL: %w", err)
	}
	d.client = redis.NewClient(opts)
	return d.client.Ping(ctx).Err()
}

func (d *RedisDriver) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

func (d *RedisDriver) Kind() DataSourceKind { return KindRedis }

// ListTables returns Redis key type categories as virtual "tables".
// It samples keys to group by type (string, list, set, hash, zset).
func (d *RedisDriver) ListTables(ctx context.Context) ([]string, error) {
	// Use SCAN to sample keys and group by type
	typeMap := make(map[string]bool)
	var cursor uint64
	for {
		keys, next, err := d.client.Scan(ctx, cursor, "*", 1000).Result()
		if err != nil {
			return nil, fmt.Errorf("scan keys: %w", err)
		}
		for _, key := range keys {
			ktype, err := d.client.Type(ctx, key).Result()
			if err != nil {
				continue
			}
			typeMap[ktype] = true
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	var tables []string
	for t := range typeMap {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	if len(tables) == 0 {
		tables = []string{"(empty)"}
	}
	return tables, nil
}

// LoadSchema returns virtual columns for a Redis type.
func (d *RedisDriver) LoadSchema(ctx context.Context, table string) ([]ColInfo, error) {
	switch table {
	case "string":
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "value", Type: "string"},
			{Name: "ttl", Type: "integer"},
		}, nil
	case "hash":
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "field", Type: "string", NotNull: true, PK: true},
			{Name: "value", Type: "string"},
			{Name: "ttl", Type: "integer"},
		}, nil
	case "list":
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "index", Type: "integer", NotNull: true, PK: true},
			{Name: "value", Type: "string"},
			{Name: "ttl", Type: "integer"},
		}, nil
	case "set":
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "member", Type: "string", NotNull: true, PK: true},
			{Name: "ttl", Type: "integer"},
		}, nil
	case "zset":
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "member", Type: "string", NotNull: true, PK: true},
			{Name: "score", Type: "float"},
			{Name: "ttl", Type: "integer"},
		}, nil
	default:
		return []ColInfo{
			{Name: "key", Type: "string", NotNull: true, PK: true},
			{Name: "value", Type: "string"},
			{Name: "ttl", Type: "integer"},
		}, nil
	}
}

func (d *RedisDriver) LoadFKs(ctx context.Context, table string) ([]FKInfo, error) {
	return nil, nil // Redis has no foreign keys
}

func (d *RedisDriver) LoadIndices(ctx context.Context, table string) ([]IndexInfo, error) {
	return nil, nil // Redis has no indices
}

// Query returns rows for a Redis type. The "query" parameter is the Redis type name.
// It samples up to PageSize keys of that type.
func (d *RedisDriver) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// Redis doesn't use sql.Rows — the app layer should call RedisQuery instead.
	// This method exists to satisfy the interface but returns an error.
	return nil, fmt.Errorf("use RedisQuery for Redis data access")
}

// RedisQuery returns formatted rows for the given Redis type.
func (d *RedisDriver) RedisQuery(ctx context.Context, redisType string) ([][]string, error) {
	var data [][]string
	var cursor uint64
	count := 0
	for {
		keys, next, err := d.client.Scan(ctx, cursor, "*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			ktype, err := d.client.Type(ctx, key).Result()
			if err != nil || ktype != redisType {
				continue
			}
			ttl := d.client.TTL(ctx, key).Val()
			ttlSec := int64(-1)
			if ttl > 0 {
				ttlSec = int64(ttl.Seconds())
			}
			switch redisType {
			case "string":
				val, err := d.client.Get(ctx, key).Result()
				if err != nil {
					continue
				}
				data = append(data, []string{key, val, fmt.Sprintf("%d", ttlSec)})
			case "hash":
				fields, err := d.client.HGetAll(ctx, key).Result()
				if err != nil {
					continue
				}
				for f, v := range fields {
					data = append(data, []string{key, f, v, fmt.Sprintf("%d", ttlSec)})
				}
			case "list":
				vals, err := d.client.LRange(ctx, key, 0, -1).Result()
				if err != nil {
					continue
				}
				for i, v := range vals {
					data = append(data, []string{key, fmt.Sprintf("%d", i), v, fmt.Sprintf("%d", ttlSec)})
				}
			case "set":
				members, err := d.client.SMembers(ctx, key).Result()
				if err != nil {
					continue
				}
				for _, m := range members {
					data = append(data, []string{key, m, fmt.Sprintf("%d", ttlSec)})
				}
			case "zset":
				members, err := d.client.ZRangeWithScores(ctx, key, 0, -1).Result()
				if err != nil {
					continue
				}
				for _, z := range members {
					data = append(data, []string{key, fmt.Sprintf("%v", z.Member), fmt.Sprintf("%.2f", z.Score), fmt.Sprintf("%d", ttlSec)})
				}
			}
			count++
			if count >= PageSize {
				break
			}
		}
		cursor = next
		if cursor == 0 || count >= PageSize {
			break
		}
	}
	return data, nil
}

func (d *RedisDriver) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, fmt.Errorf("use Redis-specific commands for mutations")
}

func (d *RedisDriver) Placeholder(idx int) string { return "?" }

func (d *RedisDriver) QuoteIdent(name string) string { return name }

func (d *RedisDriver) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

func (d *RedisDriver) DB() *sql.DB { return nil }

func (d *RedisDriver) LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error) {
	schema, _ := d.LoadSchema(ctx, table)
	cols = ColNames(schema)

	allRows, err := d.RedisQuery(ctx, table)
	if err != nil {
		return nil, nil, 0, err
	}

	total = len(allRows)

	// Apply pagination
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if offset >= total {
		return cols, nil, total, nil
	}
	if end > total {
		end = total
	}

	return cols, allRows[offset:end], total, nil
}

func (d *RedisDriver) RowCount(ctx context.Context, table string) (int, error) {
	var count int
	var cursor uint64
	for {
		keys, next, err := d.client.Scan(ctx, cursor, "*", 1000).Result()
		if err != nil {
			return 0, err
		}
		for _, key := range keys {
			ktype, err := d.client.Type(ctx, key).Result()
			if err == nil && ktype == table {
				count++
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count, nil
}

// InsertEntry inserts one logical row into a Redis type view.
func (d *RedisDriver) InsertEntry(ctx context.Context, redisType string, cols []string, vals []string) (int64, error) {
	if len(cols) == 0 {
		return 0, fmt.Errorf("no columns provided")
	}

	row := make(map[string]string, len(cols))
	for i, c := range cols {
		if i < len(vals) {
			row[c] = strings.TrimSpace(vals[i])
		} else {
			row[c] = "NULL"
		}
	}

	key := row["key"]
	if isNullish(key) {
		return 0, fmt.Errorf("key is required")
	}

	ttlSec := int64(-1)
	if ttlRaw, ok := row["ttl"]; ok && !isNullish(ttlRaw) {
		n, err := strconv.ParseInt(ttlRaw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid ttl value: %q", ttlRaw)
		}
		ttlSec = n
	}

	var affected int64
	switch redisType {
	case "string":
		value := row["value"]
		if isNullish(value) {
			value = ""
		}
		if err := d.client.Set(ctx, key, value, 0).Err(); err != nil {
			return 0, err
		}
		affected = 1

	case "hash":
		field := row["field"]
		if isNullish(field) {
			return 0, fmt.Errorf("field is required for hash")
		}
		value := row["value"]
		if isNullish(value) {
			value = ""
		}
		n, err := d.client.HSet(ctx, key, field, value).Result()
		if err != nil {
			return 0, err
		}
		affected = n

	case "list":
		value := row["value"]
		if isNullish(value) {
			value = ""
		}
		n, err := d.client.RPush(ctx, key, value).Result()
		if err != nil {
			return 0, err
		}
		if n > 0 {
			affected = 1
		}

	case "set":
		member := row["member"]
		if isNullish(member) {
			return 0, fmt.Errorf("member is required for set")
		}
		n, err := d.client.SAdd(ctx, key, member).Result()
		if err != nil {
			return 0, err
		}
		affected = n

	case "zset":
		member := row["member"]
		if isNullish(member) {
			return 0, fmt.Errorf("member is required for zset")
		}
		scoreRaw := row["score"]
		score := 0.0
		if !isNullish(scoreRaw) {
			f, err := strconv.ParseFloat(scoreRaw, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid score value: %q", scoreRaw)
			}
			score = f
		}
		n, err := d.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Result()
		if err != nil {
			return 0, err
		}
		affected = n

	default:
		return 0, fmt.Errorf("unsupported redis type: %s", redisType)
	}

	if ttlSec >= 0 {
		if err := d.client.Expire(ctx, key, time.Duration(ttlSec)*time.Second).Err(); err != nil {
			return affected, err
		}
	}

	if affected < 1 {
		affected = 1
	}

	return affected, nil
}

func isNullish(s string) bool {
	v := strings.TrimSpace(s)
	return v == "" || strings.EqualFold(v, "NULL")
}

// ExecuteQuery supports Redis command execution for the query view.
// Supported commands include GET, SET, DEL, KEYS, TYPE, TTL,
// HGETALL, LRANGE, SMEMBERS, and ZRANGE.
func (d *RedisDriver) ExecuteQuery(ctx context.Context, query string) (cols []string, rows [][]string, affected int64, err error) {
	parts := strings.Fields(strings.TrimSpace(query))
	if len(parts) == 0 {
		return nil, nil, 0, fmt.Errorf("empty query")
	}

	cmd := strings.ToUpper(parts[0])

	switch cmd {
	case "GET":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: GET <key>")
		}
		v, e := d.client.Get(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		return []string{"value"}, [][]string{{v}}, 1, nil

	case "SET":
		if len(parts) < 3 {
			return nil, nil, 0, fmt.Errorf("usage: SET <key> <value>")
		}
		if e := d.client.Set(ctx, parts[1], strings.Join(parts[2:], " "), 0).Err(); e != nil {
			return nil, nil, 0, e
		}
		return nil, nil, 1, nil

	case "DEL":
		if len(parts) < 2 {
			return nil, nil, 0, fmt.Errorf("usage: DEL <key> [key ...]")
		}
		keys := make([]string, 0, len(parts)-1)
		keys = append(keys, parts[1:]...)
		n, e := d.client.Del(ctx, keys...).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		return nil, nil, n, nil

	case "KEYS":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: KEYS <pattern>")
		}
		keys, e := d.client.Keys(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []string{k})
		}
		return []string{"key"}, rows, int64(len(rows)), nil

	case "TYPE":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: TYPE <key>")
		}
		t, e := d.client.Type(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		return []string{"type"}, [][]string{{t}}, 1, nil

	case "TTL":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: TTL <key>")
		}
		ttl, e := d.client.TTL(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		return []string{"ttl"}, [][]string{{fmt.Sprintf("%d", int64(ttl.Seconds()))}}, 1, nil

	case "HGETALL":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: HGETALL <key>")
		}
		m, e := d.client.HGetAll(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []string{k, m[k]})
		}
		return []string{"field", "value"}, rows, int64(len(rows)), nil

	case "LRANGE":
		if len(parts) != 4 {
			return nil, nil, 0, fmt.Errorf("usage: LRANGE <key> <start> <stop>")
		}
		start, e1 := strconv.ParseInt(parts[2], 10, 64)
		stop, e2 := strconv.ParseInt(parts[3], 10, 64)
		if e1 != nil || e2 != nil {
			return nil, nil, 0, fmt.Errorf("invalid LRANGE bounds")
		}
		vals, e := d.client.LRange(ctx, parts[1], start, stop).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		rows := make([][]string, 0, len(vals))
		for i, v := range vals {
			rows = append(rows, []string{fmt.Sprintf("%d", i), v})
		}
		return []string{"index", "value"}, rows, int64(len(rows)), nil

	case "SMEMBERS":
		if len(parts) != 2 {
			return nil, nil, 0, fmt.Errorf("usage: SMEMBERS <key>")
		}
		vals, e := d.client.SMembers(ctx, parts[1]).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		rows := make([][]string, 0, len(vals))
		for _, v := range vals {
			rows = append(rows, []string{v})
		}
		return []string{"member"}, rows, int64(len(rows)), nil

	case "ZRANGE":
		if len(parts) < 4 {
			return nil, nil, 0, fmt.Errorf("usage: ZRANGE <key> <start> <stop> [WITHSCORES]")
		}
		start, e1 := strconv.ParseInt(parts[2], 10, 64)
		stop, e2 := strconv.ParseInt(parts[3], 10, 64)
		if e1 != nil || e2 != nil {
			return nil, nil, 0, fmt.Errorf("invalid ZRANGE bounds")
		}
		withScores := len(parts) > 4 && strings.EqualFold(parts[4], "WITHSCORES")
		if withScores {
			zs, e := d.client.ZRangeWithScores(ctx, parts[1], start, stop).Result()
			if e != nil {
				return nil, nil, 0, e
			}
			rows := make([][]string, 0, len(zs))
			for _, z := range zs {
				rows = append(rows, []string{fmt.Sprintf("%v", z.Member), fmt.Sprintf("%v", z.Score)})
			}
			return []string{"member", "score"}, rows, int64(len(rows)), nil
		}
		vals, e := d.client.ZRange(ctx, parts[1], start, stop).Result()
		if e != nil {
			return nil, nil, 0, e
		}
		rows := make([][]string, 0, len(vals))
		for _, v := range vals {
			rows = append(rows, []string{v})
		}
		return []string{"member"}, rows, int64(len(rows)), nil
	}

	return nil, nil, 0, fmt.Errorf("unsupported Redis command: %s", cmd)
}
