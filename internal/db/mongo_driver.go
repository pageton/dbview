package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDriver implements Driver for MongoDB databases.
// It maps collections to tables and documents to rows.
type MongoDriver struct {
	client *mongo.Client
	dbName string
}

func (d *MongoDriver) Open(ctx context.Context, dsn string) error {
	opts := options.Client().ApplyURI(dsn).SetServerSelectionTimeout(10 * time.Second)
	var err error
	d.client, err = mongo.Connect(ctx, opts)
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	// Extract database name from URI
	u := dsn
	if idx := strings.Index(u, "?"); idx >= 0 {
		u = u[:idx]
	}
	parts := strings.Split(u, "/")
	if len(parts) > 3 {
		d.dbName = parts[len(parts)-1]
	} else {
		d.dbName = "test"
	}
	return d.client.Ping(ctx, nil)
}

func (d *MongoDriver) Close() error {
	if d.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.client.Disconnect(ctx)
	}
	return nil
}

func (d *MongoDriver) Kind() DataSourceKind { return KindMongoDB }

func (d *MongoDriver) ListTables(ctx context.Context) ([]string, error) {
	cursor, err := d.client.Database(d.dbName).ListCollections(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var names []string
	for cursor.Next(ctx) {
		var result struct {
			Name string `bson:"name"`
		}
		if cursor.Decode(&result) == nil {
			names = append(names, result.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// LoadSchema infers column metadata by sampling a document from the collection.
func (d *MongoDriver) LoadSchema(ctx context.Context, table string) ([]ColInfo, error) {
	coll := d.client.Database(d.dbName).Collection(table)
	doc := bson.M{}
	err := coll.FindOne(ctx, bson.M{}).Decode(&doc)
	if err != nil {
		return []ColInfo{
			{Name: "_id", Type: "ObjectId", NotNull: true, PK: true},
		}, nil
	}
	fieldTypes := make(map[string]string)
	for k, v := range doc {
		switch v.(type) {
		case string:
			fieldTypes[k] = "string"
		case int, int32, int64:
			fieldTypes[k] = "int"
		case float64:
			fieldTypes[k] = "float"
		case bool:
			fieldTypes[k] = "bool"
		case []interface{}:
			fieldTypes[k] = "array"
		case bson.M:
			fieldTypes[k] = "object"
		default:
			fieldTypes[k] = "mixed"
		}
	}
	var cols []ColInfo
	if _, ok := fieldTypes["_id"]; ok {
		cols = append(cols, ColInfo{Name: "_id", Type: fieldTypes["_id"], NotNull: true, PK: true})
		delete(fieldTypes, "_id")
	}
	var fields []string
	for f := range fieldTypes {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		cols = append(cols, ColInfo{Name: f, Type: fieldTypes[f]})
	}
	return cols, nil
}

func (d *MongoDriver) LoadFKs(ctx context.Context, table string) ([]FKInfo, error) {
	return nil, nil
}

func (d *MongoDriver) LoadIndices(ctx context.Context, table string) ([]IndexInfo, error) {
	coll := d.client.Database(d.dbName).Collection(table)
	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var indices []IndexInfo
	for cursor.Next(ctx) {
		var idxDoc struct {
			Name   string   `bson:"name"`
			Key    bson.M   `bson:"key"`
			Unique bool     `bson:"unique"`
		}
		if cursor.Decode(&idxDoc) != nil {
			continue
		}
		var cols []string
		for k := range idxDoc.Key {
			cols = append(cols, k)
		}
		indices = append(indices, IndexInfo{
			Name:    idxDoc.Name,
			Columns: cols,
			Unique:  idxDoc.Unique,
		})
	}
	return indices, nil
}

func (d *MongoDriver) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("use MongoQuery for MongoDB data access")
}

// MongoQuery returns documents from a collection as string rows.
func (d *MongoDriver) MongoQuery(ctx context.Context, collection string, filter bson.M, limit int64) ([][]string, []string, error) {
	coll := d.client.Database(d.dbName).Collection(collection)
	if filter == nil {
		filter = bson.M{}
	}
	opts := options.Find().SetLimit(limit)
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, err
	}
	if len(docs) == 0 {
		return nil, nil, nil
	}

	colSet := make(map[string]bool)
	for _, doc := range docs {
		for k := range doc {
			colSet[k] = true
		}
	}
	var columns []string
	if colSet["_id"] {
		columns = append(columns, "_id")
		delete(colSet, "_id")
	}
	var extra []string
	for k := range colSet {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	columns = append(columns, extra...)

	var data [][]string
	for _, doc := range docs {
		row := make([]string, len(columns))
		for i, col := range columns {
			val, ok := doc[col]
			if !ok {
				row[i] = ""
				continue
			}
			row[i] = FormatValue(val)
		}
		data = append(data, row)
	}
	return data, columns, nil
}

func (d *MongoDriver) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, fmt.Errorf("use Mongo-specific methods for mutations")
}

func (d *MongoDriver) Placeholder(idx int) string { return "?" }

func (d *MongoDriver) QuoteIdent(name string) string { return name }

func (d *MongoDriver) Ping(ctx context.Context) error {
	return d.client.Ping(ctx, nil)
}

func (d *MongoDriver) DB() *sql.DB { return nil }

func (d *MongoDriver) LoadTableData(ctx context.Context, table string, page, pageSize int) (cols []string, rows [][]string, total int, err error) {
	coll := d.client.Database(d.dbName).Collection(table)

	// Count total
	countOpts := options.Count()
	total64, cerr := coll.CountDocuments(ctx, bson.M{}, countOpts)
	if cerr != nil {
		total = 0
	} else {
		total = int(total64)
	}

	// Fetch page
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	findOpts := options.Find().SetSkip(skip).SetLimit(limit)
	cursor, err := coll.Find(ctx, bson.M{}, findOpts)
	if err != nil {
		return nil, nil, 0, err
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, 0, err
	}
	if len(docs) == 0 {
		schema, _ := d.LoadSchema(ctx, table)
		return ColNames(schema), nil, total, nil
	}

	// Determine columns from all docs
	colSet := make(map[string]bool)
	for _, doc := range docs {
		for k := range doc {
			colSet[k] = true
		}
	}
	if colSet["_id"] {
		cols = append(cols, "_id")
		delete(colSet, "_id")
	}
	var extra []string
	for k := range colSet {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	cols = append(cols, extra...)

	for _, doc := range docs {
		row := make([]string, len(cols))
		for i, col := range cols {
			val, ok := doc[col]
			if !ok {
				row[i] = ""
				continue
			}
			row[i] = FormatValue(val)
		}
		rows = append(rows, row)
	}

	return cols, rows, total, nil
}

func (d *MongoDriver) RowCount(ctx context.Context, table string) (int, error) {
	coll := d.client.Database(d.dbName).Collection(table)
	n64, err := coll.EstimatedDocumentCount(ctx)
	if err != nil {
		return 0, err
	}
	return int(n64), nil
}

// InsertDocument inserts one document into a MongoDB collection.
// Values are mapped by columns and loosely parsed from strings.
func (d *MongoDriver) InsertDocument(ctx context.Context, collection string, cols []string, vals []string) (int64, error) {
	if len(cols) == 0 {
		return 0, fmt.Errorf("no columns provided")
	}

	doc := bson.M{}
	for i, col := range cols {
		value := "NULL"
		if i < len(vals) {
			value = vals[i]
		}
		value = strings.TrimSpace(value)

		if col == "_id" && (value == "" || strings.EqualFold(value, "NULL")) {
			continue
		}

		parsed := parseMongoInputValue(value)
		if col == "_id" {
			if s, ok := parsed.(string); ok {
				if oid, err := primitive.ObjectIDFromHex(s); err == nil {
					doc[col] = oid
					continue
				}
			}
		}

		doc[col] = parsed
	}

	if len(doc) == 0 {
		return 0, fmt.Errorf("no fields to insert")
	}

	_, err := d.client.Database(d.dbName).Collection(collection).InsertOne(ctx, doc)
	if err != nil {
		return 0, err
	}

	return 1, nil
}

func parseMongoInputValue(raw string) interface{} {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "NULL") {
		return nil
	}

	if strings.EqualFold(v, "true") {
		return true
	}
	if strings.EqualFold(v, "false") {
		return false
	}

	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}

	if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
		var js interface{}
		if err := json.Unmarshal([]byte(v), &js); err == nil {
			return js
		}
	}

	return v
}

// ExecuteQuery supports lightweight MongoDB query commands for the query view.
// Supported forms:
// - collections
// - find <collection> [<json-filter>]
// - count <collection> [<json-filter>]
// - <json-filter> (uses defaultCollection)
func (d *MongoDriver) ExecuteQuery(ctx context.Context, query string, defaultCollection string, limit int64) (cols []string, rows [][]string, affected int64, err error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil, 0, fmt.Errorf("empty query")
	}

	lower := strings.ToLower(q)

	if lower == "collections" || lower == "show collections" {
		names, lerr := d.ListTables(ctx)
		if lerr != nil {
			return nil, nil, 0, lerr
		}
		out := make([][]string, 0, len(names))
		for _, n := range names {
			out = append(out, []string{n})
		}
		return []string{"collection"}, out, int64(len(out)), nil
	}

	if strings.HasPrefix(lower, "count ") {
		rest := strings.TrimSpace(q[len("count "):])
		parts := strings.SplitN(rest, " ", 2)
		collection := strings.TrimSpace(parts[0])
		if collection == "" {
			return nil, nil, 0, fmt.Errorf("usage: count <collection> [json-filter]")
		}
		filter := bson.M{}
		if len(parts) == 2 {
			filter, err = parseMongoFilter(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, nil, 0, err
			}
		}
		n, cerr := d.client.Database(d.dbName).Collection(collection).CountDocuments(ctx, filter)
		if cerr != nil {
			return nil, nil, 0, cerr
		}
		return []string{"count"}, [][]string{{fmt.Sprintf("%d", n)}}, int64(n), nil
	}

	collection := defaultCollection
	filter := bson.M{}

	if strings.HasPrefix(lower, "find ") {
		rest := strings.TrimSpace(q[len("find "):])
		parts := strings.SplitN(rest, " ", 2)
		collection = strings.TrimSpace(parts[0])
		if collection == "" {
			return nil, nil, 0, fmt.Errorf("usage: find <collection> [json-filter]")
		}
		if len(parts) == 2 {
			filter, err = parseMongoFilter(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, nil, 0, err
			}
		}
	} else if strings.HasPrefix(q, "{") {
		if strings.TrimSpace(collection) == "" {
			return nil, nil, 0, fmt.Errorf("no active collection; use: find <collection> <json-filter>")
		}
		filter, err = parseMongoFilter(q)
		if err != nil {
			return nil, nil, 0, err
		}
	} else {
		return nil, nil, 0, fmt.Errorf("unsupported Mongo query. Use: collections, find, count, or JSON filter")
	}

	data, outCols, qerr := d.MongoQuery(ctx, collection, filter, limit)
	if qerr != nil {
		return nil, nil, 0, qerr
	}
	return outCols, data, int64(len(data)), nil
}

func parseMongoFilter(raw string) (bson.M, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return bson.M{}, nil
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, fmt.Errorf("invalid JSON filter: %w", err)
	}
	return bson.M(obj), nil
}
