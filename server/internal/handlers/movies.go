package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"swatch/internal/database"
	"swatch/internal/models"
)

var (
	imdbIDPattern    = regexp.MustCompile(`^tt\d{7,9}$`)
	youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

func (h *Handler) ListGenres(c *gin.Context) {
	cur, err := h.db.Collection(database.GenresCollection).
		Find(c, bson.M{}, options.Find().SetSort(bson.D{{Key: "genre_name", Value: 1}}))
	if err != nil {
		internalError(c, err)
		return
	}
	genres := []models.Genre{}
	if err := cur.All(c, &genres); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, genres)
}

func (h *Handler) ListRankings(c *gin.Context) {
	rankings, err := h.rankings(c)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, rankings)
}

// ListMovies supports ?q= title search, ?genre=<genre_id>, ?page= and ?limit=.
func (h *Handler) ListMovies(c *gin.Context) {
	filter := bson.M{}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		filter["title"] = bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
	}
	if g := c.Query("genre"); g != "" {
		id, err := strconv.Atoi(g)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "genre must be a numeric genre_id"})
			return
		}
		filter["genre.genre_id"] = id
	}

	page := clampInt(c.Query("page"), 1, 1, 10_000)
	limit := clampInt(c.Query("limit"), 24, 1, 100)

	coll := h.db.Collection(database.MoviesCollection)
	total, err := coll.CountDocuments(c, filter)
	if err != nil {
		internalError(c, err)
		return
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "ranking.ranking_value", Value: -1}, {Key: "title", Value: 1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))
	cur, err := coll.Find(c, filter, opts)
	if err != nil {
		internalError(c, err)
		return
	}
	movies := []models.Movie{}
	if err := cur.All(c, &movies); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"movies": movies, "total": total, "page": page, "limit": limit})
}

func (h *Handler) GetMovie(c *gin.Context) {
	movie, err := h.findMovie(c, c.Param("imdb_id"))
	if isNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, movie)
}

type createMovieRequest struct {
	ImdbID      string         `json:"imdb_id" binding:"required"`
	Title       string         `json:"title" binding:"required,min=1,max=300"`
	Year        int            `json:"year" binding:"required,min=1888,max=2100"`
	Overview    string         `json:"overview" binding:"max=2000"`
	PosterPath  string         `json:"poster_path" binding:"omitempty,url"`
	YouTubeID   string         `json:"youtube_id" binding:"required"`
	Genre       []models.Genre `json:"genre" binding:"required,min=1,dive"`
	AdminReview string         `json:"admin_review" binding:"max=2000"`
}

func (h *Handler) CreateMovie(c *gin.Context) {
	var req createMovieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	if !imdbIDPattern.MatchString(req.ImdbID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imdb_id must look like tt1234567"})
		return
	}
	if !youtubeIDPattern.MatchString(req.YouTubeID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "youtube_id must be an 11-character YouTube video id"})
		return
	}

	now := time.Now().UTC()
	movie := models.Movie{
		ImdbID:      req.ImdbID,
		Title:       strings.TrimSpace(req.Title),
		Year:        req.Year,
		Overview:    strings.TrimSpace(req.Overview),
		PosterPath:  req.PosterPath,
		YouTubeID:   req.YouTubeID,
		Genre:       req.Genre,
		AdminReview: strings.TrimSpace(req.AdminReview),
		Ranking:     models.Ranking{RankingValue: models.NotRankedValue, RankingName: models.NotRankedName},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if movie.PosterPath == "" {
		movie.PosterPath = "https://img.youtube.com/vi/" + movie.YouTubeID + "/hqdefault.jpg"
	}
	if movie.AdminReview != "" {
		if prediction, err := h.models.Classifier.Classify(movie.AdminReview); err == nil {
			movie.Ranking = prediction.Ranking
		} else {
			log.Printf("create movie %s: review left unranked: %v", movie.ImdbID, err)
		}
	}

	res, err := h.db.Collection(database.MoviesCollection).InsertOne(c, movie)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "a movie with this imdb_id already exists"})
			return
		}
		internalError(c, err)
		return
	}
	if id, ok := res.InsertedID.(bson.ObjectID); ok {
		movie.ID = id
	}
	c.JSON(http.StatusCreated, movie)
}

type updateReviewRequest struct {
	AdminReview string `json:"admin_review" binding:"required,min=10,max=2000"`
	// RankingName lets an admin override the classifier's prediction.
	RankingName string `json:"ranking_name"`
}

// UpdateReview stores an admin review and derives the movie's ranking with the
// trained review classifier. An explicit ranking_name always wins, because the
// classifier is right about 4 times in 10 on the exact level.
func (h *Handler) UpdateReview(c *gin.Context) {
	var req updateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	var (
		ranking    models.Ranking
		confidence float64
		source     = "model"
	)
	if req.RankingName != "" {
		var err error
		if ranking, err = h.manualRanking(c, req.RankingName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown ranking_name"})
			return
		}
		source = "manual"
	} else {
		prediction, err := h.models.Classifier.Classify(req.AdminReview)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "could not rank this review; provide a ranking_name",
			})
			return
		}
		ranking, confidence = prediction.Ranking, prediction.Confidence
	}

	var movie models.Movie
	err := h.db.Collection(database.MoviesCollection).FindOneAndUpdate(c,
		bson.M{"imdb_id": c.Param("imdb_id")},
		bson.M{"$set": bson.M{
			"admin_review": strings.TrimSpace(req.AdminReview),
			"ranking":      ranking,
			"updated_at":   time.Now().UTC(),
		}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&movie)
	if isNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"movie":          movie,
		"ranking_source": source,
		"confidence":     confidence,
		"model":          h.models.Classifier.Version,
	})
}

func (h *Handler) DeleteMovie(c *gin.Context) {
	res, err := h.db.Collection(database.MoviesCollection).DeleteOne(c, bson.M{"imdb_id": c.Param("imdb_id")})
	if err != nil {
		internalError(c, err)
		return
	}
	if res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) findMovie(ctx context.Context, imdbID string) (*models.Movie, error) {
	var movie models.Movie
	err := h.db.Collection(database.MoviesCollection).FindOne(ctx, bson.M{"imdb_id": imdbID}).Decode(&movie)
	if err != nil {
		return nil, err
	}
	return &movie, nil
}

// rankings returns the rankable levels, excluding the "Not Ranked" placeholder.
func (h *Handler) rankings(ctx context.Context) ([]models.Ranking, error) {
	cur, err := h.db.Collection(database.RankingsCollection).Find(ctx,
		bson.M{"ranking_value": bson.M{"$gt": models.NotRankedValue}},
		options.Find().SetSort(bson.D{{Key: "ranking_value", Value: -1}}))
	if err != nil {
		return nil, err
	}
	rankings := []models.Ranking{}
	if err := cur.All(ctx, &rankings); err != nil {
		return nil, err
	}
	return rankings, nil
}

// PreviewReview classifies a draft review without saving it, so the admin can
// see what the model thinks before committing.
func (h *Handler) PreviewReview(c *gin.Context) {
	var req struct {
		AdminReview string `json:"admin_review" binding:"required,min=10,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	prediction, err := h.models.Classifier.Classify(req.AdminReview)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not rank this review"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prediction": prediction, "model": h.models.Classifier.Version})
}

func (h *Handler) manualRanking(ctx context.Context, name string) (models.Ranking, error) {
	rankings, err := h.rankings(ctx)
	if err != nil {
		return models.Ranking{}, err
	}
	for _, r := range rankings {
		if strings.EqualFold(r.RankingName, strings.TrimSpace(name)) {
			return r, nil
		}
	}
	return models.Ranking{}, errors.New("unknown ranking")
}

func clampInt(raw string, fallback, lo, hi int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return min(max(n, lo), hi)
}
