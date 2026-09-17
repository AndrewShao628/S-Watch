// Package middleware contains Gin middleware for authentication and authorisation.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"swatch/internal/auth"
)

const claimsKey = "claims"

// RequireAuth validates a "Bearer <access token>" Authorization header.
func RequireAuth(tm *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := tm.ParseAccess(strings.TrimSpace(token))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

// RequireRole must run after RequireAuth.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := Claims(c)
		if claims == nil || claims.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}

// Claims returns the authenticated user's claims, or nil on unauthenticated routes.
func Claims(c *gin.Context) *auth.Claims {
	v, ok := c.Get(claimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*auth.Claims)
	return claims
}
