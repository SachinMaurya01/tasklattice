package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"tasklattice/internal/config"
	"tasklattice/internal/database"
	"tasklattice/internal/handlers"
	"tasklattice/internal/middleware"
	"tasklattice/internal/repository"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

	// Redis is auxiliary: absent means in-memory limiting, no caching,
	// and a "disabled" readiness check instead of a hard failure.
	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			log.Fatalf("Invalid REDIS_URL: %v", err)
		}
		redisClient = redis.NewClient(opt)
		if err := middleware.PingRedis(context.Background(), redisClient); err != nil {
			log.Printf("Warning: Redis unreachable, continuing degraded: %v", err)
		}
	} else {
		log.Println("Warning: REDIS_URL unset, rate limiting is single-instance")
	}
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
	}

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	authService := services.NewAuthService(userRepo, sessionRepo, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	authHandler := handlers.NewAuthHandler(authService)

	orgRepo := repository.NewOrganizationRepository(pool)
	projectRepo := repository.NewProjectRepository(pool)
	taskRepo := repository.NewTaskRepository(pool)
	commentRepo := repository.NewCommentRepository(pool)
	labelRepo := repository.NewLabelRepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	idemRepo := repository.NewIdempotencyRepository(pool)

	orgService := services.NewOrganizationService(orgRepo, redisClient)
	projectService := services.NewProjectService(projectRepo, orgRepo)
	taskService := services.NewTaskService(taskRepo, projectRepo, orgRepo, labelRepo)
	commentService := services.NewCommentService(commentRepo, taskRepo, orgRepo)
	labelService := services.NewLabelService(labelRepo, taskRepo, projectRepo, orgRepo)
	auditService := services.NewAuditService(auditRepo, orgRepo)

	orgHandler := handlers.NewOrganizationHandler(orgService)
	projectHandler := handlers.NewProjectHandler(projectService)
	taskHandler := handlers.NewTaskHandler(taskService)
	commentHandler := handlers.NewCommentHandler(commentService)
	labelHandler := handlers.NewLabelHandler(labelService)
	auditHandler := handlers.NewAuditHandler(auditService)
	idempotency := middleware.NewIdempotency(idemRepo, cfg.IdempotencyTTL)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())
	router.SetTrustedProxies(nil)

	router.GET("/health", handlers.LivenessHandler())
	router.GET("/health/live", handlers.LivenessHandler())
	router.GET("/health/ready", handlers.ReadinessHandler(pool, redisClient))

	// Distributed limits share Redis across instances; without Redis the
	// same middleware degrades to the in-memory limiter.
	authLimiter := middleware.NewRedisRateLimiter(redisClient, "rl:auth", cfg.AuthRateLimitPerMin, time.Minute)
	apiLimiter := middleware.NewRedisRateLimiter(redisClient, "rl:api", cfg.AuthedRateLimitPerMin, time.Minute)

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

	// TaskLattice domain routes. All authenticated; RBAC is enforced
	// server-side in the services.
	api := v1.Group("")
	api.Use(middleware.AuthMiddleware(cfg))
	api.Use(apiLimiter.Middleware(middleware.UserIDKey))
	{
		api.POST("/organizations", idempotency.Middleware(), orgHandler.Create)
		api.GET("/organizations", orgHandler.List)
		api.GET("/organizations/:organizationID", orgHandler.Get)
		api.PATCH("/organizations/:organizationID", orgHandler.Update)
		api.DELETE("/organizations/:organizationID", orgHandler.Delete)
		api.GET("/organizations/:organizationID/audit-logs", auditHandler.List)

		api.POST("/organizations/:organizationID/members", orgHandler.AddMember)
		api.GET("/organizations/:organizationID/members", orgHandler.ListMembers)
		api.PATCH("/organizations/:organizationID/members/:userID", orgHandler.UpdateMember)
		api.DELETE("/organizations/:organizationID/members/:userID", orgHandler.RemoveMember)

		api.POST("/organizations/:organizationID/projects", idempotency.Middleware(), projectHandler.Create)
		api.GET("/organizations/:organizationID/projects", projectHandler.List)
		api.GET("/projects/:projectID", projectHandler.Get)
		api.PATCH("/projects/:projectID", projectHandler.Update)
		api.DELETE("/projects/:projectID", projectHandler.Delete)

		api.POST("/projects/:projectID/tasks", idempotency.Middleware(), taskHandler.Create)
		api.GET("/projects/:projectID/tasks", taskHandler.List)
		api.GET("/tasks/:taskID", taskHandler.Get)
		api.PATCH("/tasks/:taskID", taskHandler.Update)
		api.DELETE("/tasks/:taskID", taskHandler.Delete)
		api.POST("/tasks/:taskID/restore", taskHandler.Restore)

		api.POST("/tasks/:taskID/comments", idempotency.Middleware(), commentHandler.Create)
		api.GET("/tasks/:taskID/comments", commentHandler.List)
		api.PATCH("/comments/:commentID", commentHandler.Update)
		api.DELETE("/comments/:commentID", commentHandler.Delete)

		api.POST("/projects/:projectID/labels", idempotency.Middleware(), labelHandler.Create)
		api.GET("/projects/:projectID/labels", labelHandler.List)
		api.PATCH("/labels/:labelID", labelHandler.Rename)
		api.DELETE("/labels/:labelID", labelHandler.Delete)
		api.POST("/tasks/:taskID/labels", labelHandler.Attach)
		api.DELETE("/tasks/:taskID/labels/:labelID", labelHandler.Detach)
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
