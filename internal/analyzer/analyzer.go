// Package analyzer inspects a MongoDB cluster to report how much data a
// migration would transfer — used to size a migration before it is started,
// since mongosync itself reports no usable estimate until it is running.
package analyzer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Namespace selects a database and, optionally, specific collections within it.
type Namespace struct {
	Database    string   `json:"database"`
	Collections []string `json:"collections,omitempty"`
}

// DatabaseStats is the size contributed by a single database.
type DatabaseStats struct {
	Name        string `json:"name"`
	DataSize    int64  `json:"dataSize"`
	StorageSize int64  `json:"storageSize"`
	Documents   int64  `json:"documents"`
	Collections int64  `json:"collections"`
	Indexes     int64  `json:"indexes"`
}

// Result is the aggregate size of the data a migration would transfer.
type Result struct {
	Databases   []DatabaseStats `json:"databases"`
	DataSize    int64           `json:"dataSize"`
	StorageSize int64           `json:"storageSize"`
	Documents   int64           `json:"documents"`
	Collections int64           `json:"collections"`
	Indexes     int64           `json:"indexes"`
}

// systemDB reports whether a database is internal and should be skipped.
func systemDB(name string) bool {
	switch name {
	case "admin", "config", "local":
		return true
	}
	return strings.HasPrefix(name, "mongosync_")
}

// num reads a numeric BSON value, which the stats commands may return as
// int32, int64 or double depending on the field and the server.
func num(m bson.M, key string) int64 {
	switch v := m[key].(type) {
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

// Analyze connects to the cluster at uri and reports the size of the data a
// migration would transfer. With no namespaces it covers every user database;
// otherwise only the requested databases and collections.
func Analyze(ctx context.Context, uri string, include []Namespace) (*Result, error) {
	client, err := connect(ctx, uri)
	if err != nil {
		return nil, err
	}
	defer disconnect(client)

	res := &Result{Databases: []DatabaseStats{}}
	add := func(d DatabaseStats) {
		res.Databases = append(res.Databases, d)
		res.DataSize += d.DataSize
		res.StorageSize += d.StorageSize
		res.Documents += d.Documents
		res.Collections += d.Collections
		res.Indexes += d.Indexes
	}

	// No whitelist: every user database.
	if len(include) == 0 {
		names, err := client.ListDatabaseNames(ctx, bson.D{})
		if err != nil {
			return nil, fmt.Errorf("list databases: %w", err)
		}
		for _, name := range names {
			if systemDB(name) {
				continue
			}
			d, err := dbStats(ctx, client, name)
			if err != nil {
				return nil, err
			}
			add(d)
		}
		return res, nil
	}

	// Whitelist: only the requested databases / collections.
	for _, ns := range include {
		db := strings.TrimSpace(ns.Database)
		if db == "" {
			continue
		}
		if len(ns.Collections) == 0 {
			d, err := dbStats(ctx, client, db)
			if err != nil {
				return nil, err
			}
			add(d)
			continue
		}
		d := DatabaseStats{Name: db}
		for _, coll := range ns.Collections {
			coll = strings.TrimSpace(coll)
			if coll == "" {
				continue
			}
			cs := collStats(ctx, client, db, coll)
			d.DataSize += cs.DataSize
			d.StorageSize += cs.StorageSize
			d.Documents += cs.Documents
			d.Indexes += cs.Indexes
			d.Collections++
		}
		add(d)
	}
	return res, nil
}

func connect(ctx context.Context, uri string) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(15 * time.Second)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		disconnect(client)
		return nil, fmt.Errorf("cannot reach cluster: %w", err)
	}
	return client, nil
}

func disconnect(client *mongo.Client) {
	dc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.Disconnect(dc)
}

// dbStats returns the size of a whole database.
func dbStats(ctx context.Context, client *mongo.Client, name string) (DatabaseStats, error) {
	var m bson.M
	cmd := bson.D{{Key: "dbStats", Value: 1}}
	if err := client.Database(name).RunCommand(ctx, cmd).Decode(&m); err != nil {
		return DatabaseStats{}, fmt.Errorf("dbStats for %q: %w", name, err)
	}
	return DatabaseStats{
		Name:        name,
		DataSize:    num(m, "dataSize"),
		StorageSize: num(m, "storageSize"),
		Documents:   num(m, "objects"),
		Collections: num(m, "collections"),
		Indexes:     num(m, "indexes"),
	}, nil
}

// collStats returns the size of a single collection. A collection that does
// not exist (or any other error) contributes zero rather than failing.
func collStats(ctx context.Context, client *mongo.Client, db, coll string) DatabaseStats {
	var m bson.M
	cmd := bson.D{{Key: "collStats", Value: coll}}
	if err := client.Database(db).RunCommand(ctx, cmd).Decode(&m); err != nil {
		return DatabaseStats{}
	}
	return DatabaseStats{
		DataSize:    num(m, "size"),
		StorageSize: num(m, "storageSize"),
		Documents:   num(m, "count"),
		Indexes:     num(m, "nindexes"),
	}
}
