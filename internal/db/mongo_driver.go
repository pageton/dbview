package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
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
