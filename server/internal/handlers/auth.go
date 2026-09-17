package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"swatch/internal/auth"
	"swatch/internal/database"
	"swatch/internal/middleware"
	"swatch/internal/models"
)

type registerRequest struct {
	FirstName       string         `json:"first_name" binding:"required,min=1,max=100"`
	LastName        string         `json:"last_name" binding:"required,min=1,max=100"`
	Email           string         `json:"email" binding:"required,email,max=254"`
	Password        string         `json:"password" binding:"required,min=8,max=72"`
	FavouriteGenres []models.Genre `json:"favourite_genres" binding:"required,min=1,dive"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type authResponse struct {
	User *models.User `json:"user"`
	*auth.TokenPair
}

func (h *Handler) RegisterUser(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(c, err)
		return
	}
	userID, err := auth.NewUserID()
	if err != nil {
		internalError(c, err)
		return
	}

	now := time.Now().UTC()
	user := &models.User{
		UserID:          userID,
		FirstName:       strings.TrimSpace(req.FirstName),
		LastName:        strings.TrimSpace(req.LastName),
		Email:           normaliseEmail(req.Email),
		Password:        hash,
		Role:            models.RoleUser,
		FavouriteGenres: req.FavouriteGenres,
		WatchHistory:    []models.WatchEntry{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	users := h.db.Collection(database.UsersCollection)
	if _, err := users.InsertOne(c, user); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "an account with this email already exists"})
			return
		}
		internalError(c, err)
		return
	}

	h.issueTokens(c, http.StatusCreated, user)
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	var user models.User
	err := h.db.Collection(database.UsersCollection).
		FindOne(c, bson.M{"email": normaliseEmail(req.Email)}).Decode(&user)
	if err != nil && !isNotFound(err) {
		internalError(c, err)
		return
	}
	if err != nil || !auth.CheckPassword(user.Password, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	h.issueTokens(c, http.StatusOK, &user)
}

// Refresh rotates the refresh token: the presented token must match the one on record.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}

	claims, err := h.tokens.ParseRefresh(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	var user models.User
	err = h.db.Collection(database.UsersCollection).FindOne(c, bson.M{"user_id": claims.UserID}).Decode(&user)
	if err != nil && !isNotFound(err) {
		internalError(c, err)
		return
	}
	if err != nil || !auth.TokenHashMatches(req.RefreshToken, user.RefreshTokenHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token has been revoked"})
		return
	}

	h.issueTokens(c, http.StatusOK, &user)
}

func (h *Handler) Logout(c *gin.Context) {
	claims := middleware.Claims(c)
	_, err := h.db.Collection(database.UsersCollection).UpdateOne(c,
		bson.M{"user_id": claims.UserID},
		bson.M{"$unset": bson.M{"refresh_token_hash": ""}},
	)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) issueTokens(c *gin.Context, status int, user *models.User) {
	pair, err := h.tokens.Generate(user)
	if err != nil {
		internalError(c, err)
		return
	}
	_, err = h.db.Collection(database.UsersCollection).UpdateOne(c,
		bson.M{"user_id": user.UserID},
		bson.M{"$set": bson.M{"refresh_token_hash": auth.HashToken(pair.RefreshToken)}},
	)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(status, authResponse{User: user, TokenPair: pair})
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
