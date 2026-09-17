// Package seed loads reference data (genres, rankings, a starter movie
// catalogue) and an optional admin account on startup. It is idempotent.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"swatch/internal/auth"
	"swatch/internal/config"
	"swatch/internal/database"
	"swatch/internal/models"
)

//go:embed data/*.json
var dataFS embed.FS

func Run(ctx context.Context, db *database.DB, cfg *config.Config) error {
	if err := seedGenres(ctx, db); err != nil {
		return err
	}
	if err := seedRankings(ctx, db); err != nil {
		return err
	}
	if err := seedMovies(ctx, db); err != nil {
		return err
	}
	return seedAdmin(ctx, db, cfg)
}

func seedGenres(ctx context.Context, db *database.DB) error {
	var genres []models.Genre
	if err := load("data/genres.json", &genres); err != nil {
		return err
	}
	coll := db.Collection(database.GenresCollection)
	for _, g := range genres {
		_, err := coll.UpdateOne(ctx, bson.M{"genre_id": g.GenreID},
			bson.M{"$set": g}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return fmt.Errorf("seed genre %q: %w", g.GenreName, err)
		}
	}
	return nil
}

func seedRankings(ctx context.Context, db *database.DB) error {
	var rankings []models.Ranking
	if err := load("data/rankings.json", &rankings); err != nil {
		return err
	}
	coll := db.Collection(database.RankingsCollection)
	for _, r := range rankings {
		_, err := coll.UpdateOne(ctx, bson.M{"ranking_name": r.RankingName},
			bson.M{"$set": r}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return fmt.Errorf("seed ranking %q: %w", r.RankingName, err)
		}
	}
	return nil
}

// seedMovies only runs against an empty catalogue so admin edits are never overwritten.
func seedMovies(ctx context.Context, db *database.DB) error {
	coll := db.Collection(database.MoviesCollection)
	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	var movies []models.Movie
	if err := load("data/movies.json", &movies); err != nil {
		return err
	}
	now := time.Now().UTC()
	docs := make([]any, 0, len(movies))
	for _, m := range movies {
		if m.PosterPath == "" {
			m.PosterPath = "https://img.youtube.com/vi/" + m.YouTubeID + "/hqdefault.jpg"
		}
		m.CreatedAt, m.UpdatedAt = now, now
		docs = append(docs, m)
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("seed movies: %w", err)
	}
	log.Printf("seeded %d movies", len(docs))
	return nil
}

// seedAdmin creates the admin account from ADMIN_EMAIL / ADMIN_PASSWORD if it doesn't exist.
func seedAdmin(ctx context.Context, db *database.DB, cfg *config.Config) error {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return nil
	}
	coll := db.Collection(database.UsersCollection)
	err := coll.FindOne(ctx, bson.M{"email": cfg.AdminEmail}).Err()
	if err == nil {
		return nil
	}
	if err != mongo.ErrNoDocuments {
		return err
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	userID, err := auth.NewUserID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = coll.InsertOne(ctx, models.User{
		UserID:    userID,
		FirstName: "Admin",
		LastName:  "User",
		Email:     cfg.AdminEmail,
		Password:  hash,
		Role:      models.RoleAdmin,
		FavouriteGenres: []models.Genre{
			{GenreID: 6, GenreName: "Drama"},
			{GenreID: 12, GenreName: "Sci-Fi"},
		},
		WatchHistory: []models.WatchEntry{},
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	log.Printf("created admin account %s", cfg.AdminEmail)
	return nil
}

func load(path string, v any) error {
	b, err := dataFS.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
