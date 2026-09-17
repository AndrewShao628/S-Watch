// Package database manages the MongoDB connection and collection indexes.
package database

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

const (
	MoviesCollection   = "movies"
	UsersCollection    = "users"
	GenresCollection   = "genres"
	RankingsCollection = "rankings"
)

type DB struct {
	Client *mongo.Client
	*mongo.Database
}

// Connect opens a client, verifies connectivity and returns the named database.
func Connect(ctx context.Context, uri, name string) (*DB, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1))

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Client: client, Database: client.Database(name)}, nil
}

func (db *DB) Close(ctx context.Context) error {
	return db.Client.Disconnect(ctx)
}

// EnsureIndexes creates the unique and lookup indexes the API relies on.
func (db *DB) EnsureIndexes(ctx context.Context) error {
	indexes := map[string][]mongo.IndexModel{
		MoviesCollection: {
			{Keys: bson.D{{Key: "imdb_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "genre.genre_id", Value: 1}}},
			{Keys: bson.D{{Key: "ranking.ranking_value", Value: -1}, {Key: "title", Value: 1}}},
		},
		UsersCollection: {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		GenresCollection: {
			{Keys: bson.D{{Key: "genre_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		RankingsCollection: {
			{Keys: bson.D{{Key: "ranking_name", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
	}
	for coll, models := range indexes {
		if _, err := db.Collection(coll).Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("indexes for %s: %w", coll, err)
		}
	}
	return nil
}
