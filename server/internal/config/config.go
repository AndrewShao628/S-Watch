// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	GinMode        string
	MongoURI       string
	DatabaseName   string
	AccessSecret   string
	RefreshSecret  string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	AllowedOrigins []string

	OpenAIKey   string
	OpenAIModel string

	RecommendationLimit int

	AdminEmail    string
	AdminPassword string
}

// Load reads configuration from the environment (and a local .env file when present).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		GinMode:             getEnv("GIN_MODE", "debug"),
		MongoURI:            getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		DatabaseName:        getEnv("DATABASE_NAME", "swatch"),
		AccessSecret:        os.Getenv("SECRET_KEY"),
		RefreshSecret:       os.Getenv("REFRESH_SECRET_KEY"),
		AccessTTL:           getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:          getDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		AllowedOrigins:      splitList(getEnv("ALLOWED_ORIGINS", "http://localhost:5173")),
		OpenAIKey:           os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		RecommendationLimit: getInt("RECOMMENDATION_LIMIT", 8),
		AdminEmail:          strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))),
		AdminPassword:       os.Getenv("ADMIN_PASSWORD"),
	}

	if cfg.AccessSecret == "" || cfg.RefreshSecret == "" {
		if cfg.GinMode == "release" {
			return nil, errors.New("SECRET_KEY and REFRESH_SECRET_KEY must be set in release mode")
		}
		log.Println("WARNING: using insecure development JWT secrets; set SECRET_KEY and REFRESH_SECRET_KEY")
		if cfg.AccessSecret == "" {
			cfg.AccessSecret = "dev-access-secret-change-me"
		}
		if cfg.RefreshSecret == "" {
			cfg.RefreshSecret = "dev-refresh-secret-change-me"
		}
	}
	if cfg.AccessSecret == cfg.RefreshSecret {
		return nil, errors.New("SECRET_KEY and REFRESH_SECRET_KEY must differ")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return nil, errors.New("ALLOWED_ORIGINS must contain at least one origin")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimRight(strings.TrimSpace(part), "/"); p != "" {
			out = append(out, p)
		}
	}
	return out
}
