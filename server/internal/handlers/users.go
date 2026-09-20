package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"swatch/internal/database"
	"swatch/internal/middleware"
	"swatch/internal/models"
	"swatch/internal/recommend"
)

const maxWatchHistory = 50

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
// This is the feedback the recommender learns from on the next training run.
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

// Recommendations folds the viewer into the trained latent space and blends the
// model's affinity scores with editorial and freshness signals.
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
	ranked := recommend.Rank(user, movies, h.models.Recommender, recommend.DefaultWeights, time.Now().UTC())

	c.JSON(http.StatusOK, gin.H{
		"recommendations": ranked[:min(len(ranked), limit)],
		"model":           h.models.Recommender.Version,
		"trained_at":      h.models.Recommender.TrainedAt,
	})
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
