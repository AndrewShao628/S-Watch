// Package handlers implements the REST API endpoints.
package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"swatch/internal/ai"
	"swatch/internal/auth"
	"swatch/internal/config"
	"swatch/internal/database"
	"swatch/internal/middleware"
	"swatch/internal/models"
)

type Handler struct {
	cfg    *config.Config
	db     *database.DB
	tokens *auth.TokenManager
	ai     *ai.Service
}

func New(cfg *config.Config, db *database.DB, tokens *auth.TokenManager, aiSvc *ai.Service) *Handler {
	return &Handler{cfg: cfg, db: db, tokens: tokens, ai: aiSvc}
}

// Register wires every route onto the engine.
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/health", h.Health)

	api := r.Group("/api")
	{
		api.POST("/auth/register", h.RegisterUser)
		api.POST("/auth/login", h.Login)
		api.POST("/auth/refresh", h.Refresh)

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
		admin.DELETE("/movies/:imdb_id", h.DeleteMovie)
	}
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "ai_enabled": h.ai.Enabled()})
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
