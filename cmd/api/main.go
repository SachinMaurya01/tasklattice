package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"Todo-App/internal/config"
	"Todo-App/internal/database"
	"Todo-App/internal/handlers"
	"Todo-App/internal/middleware"
	"Todo-App/internal/repository"
	"Todo-App/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	pool, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	authService := services.NewAuthService(userRepo, sessionRepo, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	authHandler := handlers.NewAuthHandler(authService)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())
	router.SetTrustedProxies(nil)

	router.GET("/health", handlers.LivenessHandler())
	router.GET("/health/live", handlers.LivenessHandler())
	router.GET("/health/ready", handlers.ReadinessHandler(pool))

	authLimiter := middleware.NewRateLimiter(cfg.AuthRateLimitPerMin, time.Minute)

	v1 := router.Group("/api/v1")

	auth := v1.Group("/auth")
	auth.Use(authLimiter.Middleware(middleware.ClientIPKey))
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)

		authed := auth.Group("")
		authed.Use(middleware.AuthMiddleware(cfg))
		authed.POST("/logout-all", authHandler.LogoutAll)
	}

	todos := v1.Group("/todos")
	todos.Use(middleware.AuthMiddleware(cfg))
	{
		todos.POST("", handlers.CreateTodoHandler(pool))
		todos.GET("", handlers.GetAllTodosHandler(pool))
		todos.GET("/:id", handlers.GetToDoByIDHandler(pool))
		todos.PUT("/:id", handlers.UpdateToDoHandler(pool))
		todos.DELETE("/:id", handlers.DeleteToDoHandler(pool))
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("Shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}
	log.Println("Server stopped")
}
