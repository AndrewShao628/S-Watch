// Package models defines the documents stored in MongoDB and exposed over the API.
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"

	// NotRankedValue marks a movie whose admin review has not been classified yet.
	NotRankedValue = 0
	NotRankedName  = "Not Ranked"
)

type Genre struct {
	GenreID   int    `bson:"genre_id" json:"genre_id" binding:"required,min=1"`
	GenreName string `bson:"genre_name" json:"genre_name" binding:"required,min=2,max=100"`
}

// Ranking scores run from 1 (Terrible) to 5 (Excellent); 0 means not ranked.
type Ranking struct {
	RankingValue int    `bson:"ranking_value" json:"ranking_value"`
	RankingName  string `bson:"ranking_name" json:"ranking_name"`
}

type Movie struct {
	ID          bson.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ImdbID      string        `bson:"imdb_id" json:"imdb_id"`
	Title       string        `bson:"title" json:"title"`
	Year        int           `bson:"year" json:"year"`
	Overview    string        `bson:"overview" json:"overview"`
	PosterPath  string        `bson:"poster_path" json:"poster_path"`
	YouTubeID   string        `bson:"youtube_id" json:"youtube_id"`
	Genre       []Genre       `bson:"genre" json:"genre"`
	AdminReview string        `bson:"admin_review" json:"admin_review"`
	Ranking     Ranking       `bson:"ranking" json:"ranking"`
	CreatedAt   time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time     `bson:"updated_at" json:"updated_at"`
}

type WatchEntry struct {
	ImdbID    string    `bson:"imdb_id" json:"imdb_id"`
	WatchedAt time.Time `bson:"watched_at" json:"watched_at"`
}

type User struct {
	ID               bson.ObjectID `bson:"_id,omitempty" json:"-"`
	UserID           string        `bson:"user_id" json:"user_id"`
	FirstName        string        `bson:"first_name" json:"first_name"`
	LastName         string        `bson:"last_name" json:"last_name"`
	Email            string        `bson:"email" json:"email"`
	Password         string        `bson:"password" json:"-"`
	Role             string        `bson:"role" json:"role"`
	RefreshTokenHash string        `bson:"refresh_token_hash,omitempty" json:"-"`
	FavouriteGenres  []Genre       `bson:"favourite_genres" json:"favourite_genres"`
	WatchHistory     []WatchEntry  `bson:"watch_history" json:"watch_history"`
	CreatedAt        time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time     `bson:"updated_at" json:"updated_at"`
}

// Recommendation is a ranked movie suggestion with a human-readable reason.
type Recommendation struct {
	Movie  Movie   `json:"movie"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}
