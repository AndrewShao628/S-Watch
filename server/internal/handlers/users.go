package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"swatch/internal/ai"
	"swatch/internal/database"
	"swatch/internal/middleware"
	"swatch/internal/models"
	"swatch/internal/recommend"
)

const (
	maxWatchHistory = 50
	// How many heuristic top candidates are handed to the LLM for re-ranking.
	rerankPoolSize = 15
)

func (h *Handler) Me(c *gin.Context) {
	user, err := h.currentUser(c)
	if isNotFound(err) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user no longer exists"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

type updateGenresRequest struct {
	FavouriteGenres []models.Genre `json:"favourite_genres" binding:"required,min=1,dive"`
}

func (h *Handler) UpdateFavouriteGenres(c *gin.Context) {
	var req updateGenresRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	var user models.User
	err := h.db.Collection(database.UsersCollection).FindOneAndUpdate(c,
		bson.M{"user_id": middleware.Claims(c).UserID},
		bson.M{"$set": bson.M{"favourite_genres": req.FavouriteGenres, "updated_at": time.Now().UTC()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&user)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// RecordWatch appends to the user's capped watch history when playback starts.
func (h *Handler) RecordWatch(c *gin.Context) {
	imdbID := c.Param("imdb_id")
	if _, err := h.findMovie(c, imdbID); err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
			return
		}
		internalError(c, err)
		return
	}

	entry := models.WatchEntry{ImdbID: imdbID, WatchedAt: time.Now().UTC()}
	_, err := h.db.Collection(database.UsersCollection).UpdateOne(c,
		bson.M{"user_id": middleware.Claims(c).UserID},
		bson.M{"$push": bson.M{"watch_history": bson.M{
			"$each":  []models.WatchEntry{entry},
			"$slice": -maxWatchHistory,
		}}},
	)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

// Recommendations scores the catalogue with the weighted ranking algorithm and,
// when OpenAI is configured, lets the LLM re-rank and explain the top candidates.
func (h *Handler) Recommendations(c *gin.Context) {
	user, err := h.currentUser(c)
	if err != nil {
		internalError(c, err)
		return
	}

	cur, err := h.db.Collection(database.MoviesCollection).Find(c, bson.M{}, options.Find().SetLimit(1000))
	if err != nil {
		internalError(c, err)
		return
	}
	var movies []models.Movie
	if err := cur.All(c, &movies); err != nil {
		internalError(c, err)
		return
	}

	limit := clampInt(c.Query("limit"), h.cfg.RecommendationLimit, 1, 24)
	ranked := recommend.Rank(user, movies, recommend.DefaultWeights, time.Now().UTC())
	pool := ranked[:min(len(ranked), max(rerankPoolSize, limit))]

	if h.ai.Enabled() && len(pool) > 0 {
		recs, err := h.aiRerank(c, user, movies, pool, limit)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"recommendations": recs, "source": "ai"})
			return
		}
		log.Printf("ai rerank failed, falling back to heuristic: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"recommendations": pool[:min(len(pool), limit)], "source": "heuristic"})
}

func (h *Handler) aiRerank(ctx context.Context, user *models.User, movies []models.Movie, pool []models.Recommendation, limit int) ([]models.Recommendation, error) {
	titles := make(map[string]string, len(movies))
	for _, m := range movies {
		titles[m.ImdbID] = m.Title
	}

	profile := ai.Profile{FirstName: user.FirstName}
	for _, g := range user.FavouriteGenres {
		profile.FavouriteGenres = append(profile.FavouriteGenres, g.GenreName)
	}
	seen := map[string]bool{}
	for i := len(user.WatchHistory) - 1; i >= 0 && len(profile.RecentlyWatched) < 10; i-- {
		id := user.WatchHistory[i].ImdbID
		if t, ok := titles[id]; ok && !seen[id] {
			seen[id] = true
			profile.RecentlyWatched = append(profile.RecentlyWatched, t)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	picks, err := h.ai.Rerank(ctx, profile, pool, min(limit, len(pool)))
	if err != nil {
		return nil, err
	}

	byID := make(map[string]models.Recommendation, len(pool))
	for _, r := range pool {
		byID[r.Movie.ImdbID] = r
	}
	recs := make([]models.Recommendation, 0, len(picks))
	for _, p := range picks {
		r := byID[p.ImdbID]
		if p.Reason != "" {
			r.Reason = p.Reason
		}
		recs = append(recs, r)
	}
	return recs, nil
}

func (h *Handler) currentUser(c *gin.Context) (*models.User, error) {
	var user models.User
	err := h.db.Collection(database.UsersCollection).
		FindOne(c, bson.M{"user_id": middleware.Claims(c).UserID}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
