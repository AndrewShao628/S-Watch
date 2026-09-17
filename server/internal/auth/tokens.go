// Package auth issues and verifies JWT access/refresh tokens and hashes passwords.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"swatch/internal/models"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
	issuer           = "s-watch"
)

type Claims struct {
	UserID    string `json:"uid"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TokenManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewTokenManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

// Generate issues a new access/refresh token pair for the user.
func (m *TokenManager) Generate(u *models.User) (*TokenPair, error) {
	now := time.Now()
	access, err := m.sign(u, tokenTypeAccess, now, m.accessTTL, m.accessSecret)
	if err != nil {
		return nil, err
	}
	refresh, err := m.sign(u, tokenTypeRefresh, now, m.refreshTTL, m.refreshSecret)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresAt: now.Add(m.accessTTL)}, nil
}

func (m *TokenManager) ParseAccess(token string) (*Claims, error) {
	return m.parse(token, tokenTypeAccess, m.accessSecret)
}

func (m *TokenManager) ParseRefresh(token string) (*Claims, error) {
	return m.parse(token, tokenTypeRefresh, m.refreshSecret)
}

func (m *TokenManager) sign(u *models.User, typ string, now time.Time, ttl time.Duration, secret []byte) (string, error) {
	jti, err := randomID()
	if err != nil {
		return "", err
	}
	claims := Claims{
		UserID:    u.UserID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Role:      u.Role,
		TokenType: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   u.UserID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func (m *TokenManager) parse(token, typ string, secret []byte) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != typ {
		return nil, errors.New("unexpected token type")
	}
	return claims, nil
}

// HashToken returns a SHA-256 digest so raw refresh tokens are never stored.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TokenHashMatches(token, hash string) bool {
	return hash != "" && subtle.ConstantTimeCompare([]byte(HashToken(token)), []byte(hash)) == 1
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewUserID returns a random opaque identifier for a new user.
func NewUserID() (string, error) {
	return randomID()
}
