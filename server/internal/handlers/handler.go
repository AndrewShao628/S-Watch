// Package handlers implements the REST API endpoints.
package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"swatch/internal/auth"
	"swatch/internal/config"
	"swatch/internal/database"
	"swatch/internal/middleware"
	"swatch/internal/model"
	"swatch/internal/models"
)

type Handler struct {
	cfg    *config.Config
	db     *database.DB
	tokens *auth.TokenManager
	models *model.Engine
}

func New(cfg *config.Config, db *database.DB, tokens *auth.TokenManager, engine *model.Engine) *Handler {
	return &Handler{cfg: cfg, db: db, tokens: tokens, models: engine}
}

// Register wires every route onto the engine.
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/health", h.Health)

	api := r.Group("/api")
	{
		api.POST("/auth/register", h.RegisterUser)
		api.POST("/auth/login", h.Login)
		api.POST("/auth/refresh", h.Refresh)

		api.GET("/model", h.ModelInfo)
		api.GET("/genres", h.ListGenres)
		api.GET("/rankings", h.ListRankings)
		api.GET("/movies", h.ListMovies)
		api.GET("/movies/:imdb_id", h.GetMovie)
	}

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(h.tokens))
	{
		protected.POST("/auth/logout", h.Logout)
		protected.GET("/me", h.Me)
		protected.PUT("/me/genres", h.UpdateFavouriteGenres)
		protected.POST("/movies/:imdb_id/watch", h.RecordWatch)
		protected.GET("/recommendations", h.Recommendations)
	}

	admin := protected.Group("/admin")
	admin.Use(middleware.RequireRole(models.RoleAdmin))
	{
		admin.POST("/movies", h.CreateMovie)
		admin.PATCH("/movies/:imdb_id/review", h.UpdateReview)
		admin.POST("/reviews/preview", h.PreviewReview)
		admin.DELETE("/movies/:imdb_id", h.DeleteMovie)
	}
}

func (h *Handler) Health(c *gin.Context) {
	info := h.models.Info()
	c.JSON(http.StatusOK, gin.H{
		"status":            "ok",
		"recommender_model": info.Recommender.Version,
		"classifier_model":  info.Classifier.Version,
	})
}

// ModelInfo reports which trained models are serving traffic, including the
// metrics from their training runs.
func (h *Handler) ModelInfo(c *gin.Context) {
	c.JSON(http.StatusOK, h.models.Info())
}

func badRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
}

func internalError(c *gin.Context, err error) {
	log.Printf("%s %s: %v", c.Request.Method, c.FullPath(), err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}
