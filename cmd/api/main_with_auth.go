package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/passbi/passbi_core/internal/api"
	"github.com/passbi/passbi_core/internal/cache"
	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/middleware"
)

func main() {
	log.Println("Starting PassBi API server...")

	// Initialize database connection
	pool, err := db.GetDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database connection established")

	// Initialize Redis connection
	rdb, err := cache.GetClient()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer cache.Close()
	log.Println("✓ Redis connection established")

	// Check if authentication is enabled
	enableAuth := getEnvBool("ENABLE_AUTH", true)
	enableRateLimit := getEnvBool("ENABLE_RATE_LIMIT", true)
	enableAnalytics := getEnvBool("ENABLE_ANALYTICS", true)

	log.Printf("Configuration: Auth=%v, RateLimit=%v, Analytics=%v", enableAuth, enableRateLimit, enableAnalytics)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "PassBi API v2.0",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path} | ${ip}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "Local",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: false,
	}))

	// Inject dependencies into context
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("db", pool)
		c.Locals("redis", rdb)
		return c.Next()
	})

	// ============================================
	// Public Routes (no authentication required)
	// ============================================
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"name":          "PassBi Core API",
			"version":       "2.0.0",
			"documentation": "https://docs.passbi.com",
			"status":        "operational",
			"authentication": map[string]interface{}{
				"enabled": enableAuth,
				"type":    "Bearer Token (API Key)",
				"format":  "Authorization: Bearer pk_live_...",
			},
		})
	})

	app.Get("/health", api.Health)

	// ============================================
	// API V2 - Protected Routes
	// ============================================
	v2 := app.Group("/v2")

	// Apply authentication middleware if enabled
	if enableAuth {
		v2.Use(middleware.AuthMiddleware(pool))
		log.Println("✓ Authentication middleware enabled")
	}

	// Apply rate limiting middleware if enabled
	if enableRateLimit && enableAuth {
		v2.Use(middleware.RateLimitMiddleware(rdb))
		log.Println("✓ Rate limiting middleware enabled")
	}

	// Apply analytics middleware if enabled
	if enableAnalytics && enableAuth {
		v2.Use(middleware.AnalyticsMiddleware(pool))
		log.Println("✓ Analytics middleware enabled")
	}

	// Core API endpoints
	v2.Get("/route-search", api.RouteSearch)
	v2.Get("/stops/nearby", api.StopsNearby)
	v2.Get("/routes/list", api.RoutesList)

	// ============================================
	// Partner Dashboard API
	// ============================================
	if enableAuth {
		dashboard := app.Group("/dashboard")
		dashboard.Use(middleware.AuthMiddleware(pool))

		// Partner information
		dashboard.Get("/me", api.GetPartnerInfo)

		// API key management
		dashboard.Get("/api-keys", api.GetAPIKeys)
		dashboard.Post("/api-keys", api.CreateAPIKey)
		dashboard.Delete("/api-keys/:id", api.RevokeAPIKey)

		// Usage and analytics
		dashboard.Get("/usage", api.GetUsageStats)
		dashboard.Get("/quota", api.GetQuotaUsage)

		log.Println("✓ Dashboard API endpoints registered")
	}

	// ============================================
	// Admin Routes (optional - for future use)
	// ============================================
	// admin := app.Group("/admin")
	// admin.Use(middleware.AdminAuth(pool))
	// admin.Get("/partners", api.ListPartners)
	// admin.Post("/partners", api.CreatePartner)
	// admin.Put("/partners/:id", api.UpdatePartner)

	// ============================================
	// 404 handler
	// ============================================
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error":   "not_found",
			"message": "The requested endpoint does not exist",
			"path":    c.Path(),
		})
	})

	// Get port from environment
	port := getEnv("API_PORT", "8080")
	addr := fmt.Sprintf(":%s", port)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("\n⚠️  Received shutdown signal...")
		log.Println("Closing database connections...")
		db.Close()
		log.Println("Closing Redis connections...")
		cache.Close()
		log.Println("Shutting down server...")

		if err := app.ShutdownWithTimeout(30 * time.Second); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		log.Println("✓ Server shut down gracefully")
	}()

	// Start server
	log.Println("═══════════════════════════════════════════════════")
	log.Printf("🚀 PassBi API Server Started")
	log.Printf("📍 Listening on: http://localhost%s", addr)
	log.Println("═══════════════════════════════════════════════════")
	log.Println("Available Endpoints:")
	log.Printf("  GET  /                     - API information")
	log.Printf("  GET  /health               - Health check")
	log.Printf("  GET  /v2/route-search      - Route planning")
	log.Printf("  GET  /v2/stops/nearby      - Find nearby stops")
	log.Printf("  GET  /v2/routes/list       - List all routes")
	if enableAuth {
		log.Println("\nPartner Dashboard:")
		log.Printf("  GET  /dashboard/me         - Partner info")
		log.Printf("  GET  /dashboard/api-keys   - List API keys")
		log.Printf("  POST /dashboard/api-keys   - Create API key")
		log.Printf("  GET  /dashboard/usage      - Usage statistics")
		log.Printf("  GET  /dashboard/quota      - Quota status")
	}
	log.Println("═══════════════════════════════════════════════════")

	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// customErrorHandler handles errors returned from handlers
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	log.Printf("Error [%s %s]: %v", c.Method(), c.Path(), err)

	return c.Status(code).JSON(fiber.Map{
		"error":   "internal_error",
		"message": err.Error(),
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
