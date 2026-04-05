package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

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
