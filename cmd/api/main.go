//go:build !with_auth

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/passbi/passbi_core/internal/api"
	"github.com/passbi/passbi_core/internal/cache"
	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/graph"
)

func main() {
	log.Println("Starting PassBi API server...")

	// Initialize database connection
	if _, err := db.GetDB(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database connection established")

	// Initialize Redis connection
	if _, err := cache.GetClient(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer cache.Close()
	log.Println("✓ Redis connection established")

	// Load routing graph into memory
	pool, _ := db.GetDB()
	g := graph.GetGraph()
	if err := g.LoadFromDB(context.Background(), pool); err != nil {
		log.Fatalf("Failed to load routing graph: %v", err)
	}
	log.Println("✓ Routing graph loaded into memory")

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "PassBi API",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorHandler: customErrorHandler,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "Local",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Routes
	app.Get("/health", api.Health)
	app.Get("/v2/route-search", api.RouteSearch)
	app.Get("/v2/stops/nearby", api.StopsNearby)
	app.Get("/v2/stops/search", api.StopsSearch)
	app.Get("/v2/routes/list", api.RoutesList)
	app.Get("/v2/stops/:id/departures", api.StopDepartures)
	app.Get("/v2/routes/:id/schedule", api.RouteSchedule)
	app.Get("/v2/routes/:id/trips", api.RouteTrips)

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error": "endpoint not found",
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

		log.Println("Shutting down gracefully...")
		if err := app.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	// Start server
	log.Printf("🚀 Server listening on http://localhost%s", addr)
	log.Printf("📍 Route search: http://localhost%s/v2/route-search?from=LAT,LON&to=LAT,LON", addr)
	log.Printf("❤️  Health check: http://localhost%s/health", addr)

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

	log.Printf("Error: %v", err)

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
