package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"swatch/internal/auth"
	"swatch/internal/config"
	"swatch/internal/database"
	"swatch/internal/handlers"
	"swatch/internal/model"
	"swatch/internal/seed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	gin.SetMode(cfg.GinMode)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	db, err := database.Connect(ctx, cfg.MongoURI, cfg.DatabaseName)
	if err != nil {
		log.Fatalf("mongodb: %v", err)
	}
	if err := db.EnsureIndexes(ctx); err != nil {
		log.Fatalf("mongodb: %v", err)
	}
	if err := seed.Run(ctx, db, cfg); err != nil {
		log.Fatalf("seed: %v", err)
	}
	cancel()

	engine, err := model.Load(cfg.ModelDir)
	if err != nil {
		log.Fatalf("model: %v", err)
	}
	info := engine.Info()
	log.Printf("recommender: %s (trained %s)", info.Recommender.Version, info.Recommender.TrainedAt)
	log.Printf("review classifier: %s (trained %s)", info.Classifier.Version, info.Classifier.TrainedAt)

	tokens := auth.NewTokenManager(cfg.AccessSecret, cfg.RefreshSecret, cfg.AccessTTL, cfg.RefreshTTL)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("router: %v", err)
	}
	router.Use(cors.New(cors.Config{
		AllowOrigins:  cfg.AllowedOrigins,
		AllowWildcard: true, // permits entries such as https://*.vercel.app for preview deploys
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization"},
		MaxAge:        12 * time.Hour,
	}))

	handlers.New(cfg, db, tokens, engine).Register(router)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("S-Watch API listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
	if err := db.Close(shutdownCtx); err != nil {
		log.Printf("mongodb disconnect: %v", err)
	}
}
