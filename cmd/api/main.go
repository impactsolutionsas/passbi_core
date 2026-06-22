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
	"github.com/passbi/passbi_core/internal/audit"
	"github.com/passbi/passbi_core/internal/cache"
	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/graph"
	adminHandlers "github.com/passbi/passbi_core/internal/handlers/admin"
	partnerPortalHandlers "github.com/passbi/passbi_core/internal/handlers/partner_portal"
	"github.com/passbi/passbi_core/internal/middleware"
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
		ReadTimeout:  60 * time.Second,  // increased for GTFS ZIP uploads
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    100 * 1024 * 1024, // 100 MB — GTFS ZIPs can be large
		ErrorHandler: customErrorHandler,
	})

	// Get DB pool for handlers
	dbPool, _ := db.GetDB()

	// Initialize audit logger (RGPD)
	auditLogger := audit.New(dbPool)

	// Start retention worker (purge + alerting en arrière-plan)
	audit.StartRetentionWorker(context.Background(), dbPool, audit.DefaultRetention)

	// Redis client for rate limiting / brute-force
	redisClient, _ := cache.GetClient()

	// Middleware — ordre important
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(middleware.RequestID())
	app.Use(middleware.SecurityHeaders())
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${ip} | ${method} ${path} | reqid=${locals:request_id}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "UTC",
	}))
	app.Use(cors.New(cors.Config{
		// Restreindre aux origines connues en production (pas de wildcard)
		AllowOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173,http://localhost:8100"),
		AllowMethods: "GET,POST,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
	}))
	// Rate limit global par IP (100 req/min) — avant l'auth
	app.Use(middleware.IPRateLimit(redisClient, 100))
	// Détection de patterns suspects sur toutes les routes
	app.Use(middleware.SuspiciousPatternDetector(dbPool, redisClient))

	// Routes — Public transit API
	app.Get("/health", api.Health)

	// Mobile app — no API key required (public consumer app)
	mobile := app.Group("/mobile/v1", middleware.AnalyticsMiddleware(dbPool))
	mobile.Get("/route-search", api.RouteSearch)
	mobile.Get("/stops/nearby", api.StopsNearby)
	mobile.Get("/stops/search", api.StopsSearch)
	mobile.Get("/routes/list", api.RoutesList)
	mobile.Get("/stops/:id/departures", api.StopDepartures)
	mobile.Get("/routes/:id/schedule", api.RouteSchedule)
	mobile.Get("/routes/:id/trips", api.RouteTrips)

	// Partner API — API key required (B2B)
	v2 := app.Group("/v2", middleware.AuthMiddleware(dbPool), middleware.AnalyticsMiddleware(dbPool))
	v2.Get("/route-search", api.RouteSearch)
	v2.Get("/stops/nearby", api.StopsNearby)
	v2.Get("/stops/search", api.StopsSearch)
	v2.Get("/routes/list", api.RoutesList)
	v2.Get("/stops/:id/departures", api.StopDepartures)
	v2.Get("/routes/:id/schedule", api.RouteSchedule)
	v2.Get("/routes/:id/trips", api.RouteTrips)

	// Routes — Auth (public) avec brute-force protection (5 échecs → blocage 30min)
	auth := app.Group("/api/auth", middleware.BruteForceProtection(redisClient, 5))
	auth.Post("/admin/login", adminHandlers.Login(dbPool, auditLogger))
	auth.Post("/partner/login", partnerPortalHandlers.Login(dbPool, auditLogger))

	// Routes — Admin backoffice (JWT protected)
	admin := app.Group("/api/admin", middleware.AdminAuthMiddleware())
	admin.Get("/me", adminHandlers.Me())

	// Profile & password self-service (any authenticated admin)
	admin.Get("/me/profile", adminHandlers.GetMyProfile(dbPool))
	admin.Patch("/me/profile", adminHandlers.UpdateProfile(dbPool, auditLogger))
	admin.Patch("/me/password", adminHandlers.ChangePassword(dbPool, auditLogger))

	admin.Get("/dashboard/kpis", adminHandlers.DashboardKPIs(dbPool))
	admin.Get("/partners", adminHandlers.ListPartners(dbPool))
	admin.Post("/partners", adminHandlers.CreatePartner(dbPool, auditLogger))
	admin.Get("/partners/:id", adminHandlers.GetPartner(dbPool))
	admin.Patch("/partners/:id", adminHandlers.UpdatePartner(dbPool, auditLogger))
	admin.Get("/partners/:id/api-keys", adminHandlers.ListPartnerAPIKeys(dbPool))
	admin.Delete("/api-keys/:keyId", adminHandlers.RevokeAPIKey(dbPool, auditLogger))
	admin.Get("/analytics/overview", adminHandlers.AnalyticsOverview(dbPool))
	admin.Get("/tiers", adminHandlers.ListTiers(dbPool))
	admin.Patch("/tiers/:tier", adminHandlers.UpdateTier(dbPool, auditLogger))
	admin.Get("/system/health", adminHandlers.SystemHealth(dbPool))
	admin.Get("/gtfs/agencies", adminHandlers.ListAgencies(dbPool))
	admin.Post("/gtfs/agencies", adminHandlers.CreateAgency(dbPool))
	admin.Patch("/gtfs/agencies/:id", adminHandlers.UpdateAgency(dbPool))
	admin.Post("/gtfs/agencies/:agencyId/upload", adminHandlers.UploadGTFS(dbPool))
	admin.Get("/gtfs/agencies/:agencyId/versions", adminHandlers.ListVersions(dbPool))
	admin.Post("/gtfs/versions/:versionId/import", adminHandlers.ImportVersion(dbPool))
	admin.Get("/gtfs/logs", adminHandlers.ListImportLogs(dbPool))
	admin.Get("/co2/overview", adminHandlers.CO2Overview(dbPool))
	admin.Get("/co2/report", adminHandlers.CO2Report(dbPool))
	admin.Get("/co2/export", adminHandlers.CO2ExportCSV(dbPool))
	admin.Get("/audit", adminHandlers.AuditLog(dbPool))
	admin.Get("/audit/stats", adminHandlers.AuditStats(dbPool))
	admin.Get("/audit/export", adminHandlers.AuditExport(dbPool))
	admin.Get("/security/events", adminHandlers.SecurityEvents(dbPool))
	admin.Get("/security/export", adminHandlers.SecurityExport(dbPool))

	// User management — superadmin only
	superadmin := admin.Group("/users", middleware.RequireAdminRole("superadmin"))
	superadmin.Get("/", adminHandlers.ListAdminUsers(dbPool))
	superadmin.Post("/", adminHandlers.CreateAdminUser(dbPool, auditLogger))
	superadmin.Get("/:id", adminHandlers.GetAdminUser(dbPool))
	superadmin.Patch("/:id", adminHandlers.UpdateAdminUser(dbPool, auditLogger))
	superadmin.Delete("/:id", adminHandlers.DeactivateAdminUser(dbPool, auditLogger))
	superadmin.Post("/:id/reset-password", adminHandlers.ResetAdminPassword(dbPool, auditLogger))

	// Billing — superadmin + accountant (read); superadmin + admin (write)
	billing := admin.Group("/billing", middleware.RequireAnyRole("accountant"))
	billing.Get("/revenue", adminHandlers.BillingRevenue(dbPool))
	billing.Get("/invoices", adminHandlers.ListInvoices(dbPool))
	billing.Get("/invoices/:id", adminHandlers.GetInvoice(dbPool))
	billing.Get("/payments", adminHandlers.ListPayments(dbPool))
	billing.Get("/devis", adminHandlers.ListDevis(dbPool))
	billing.Get("/devis/:id", adminHandlers.GetDevis(dbPool))
	// Write operations — superadmin + admin only
	billingWrite := admin.Group("/billing", middleware.RequireAdminRole("admin"))
	billingWrite.Patch("/invoices/:id", adminHandlers.UpdateInvoice(dbPool, auditLogger))
	billingWrite.Post("/invoices/:id/send", adminHandlers.SendInvoice(dbPool, auditLogger))
	billingWrite.Post("/invoices/:id/cancel", adminHandlers.CancelInvoice(dbPool, auditLogger))
	billingWrite.Post("/invoices/:id/payments", adminHandlers.RecordPayment(dbPool, auditLogger))
	billingWrite.Post("/invoices/mark-overdue", adminHandlers.MarkOverdueInvoices(dbPool))
	billingWrite.Post("/devis/:id/send", adminHandlers.SendDevis(dbPool, auditLogger))
	billingWrite.Post("/devis/:id/respond", adminHandlers.RespondDevis(dbPool, auditLogger))
	billingWrite.Post("/devis/:id/convert", adminHandlers.ConvertDevisToInvoice(dbPool, auditLogger))
	// Auto-generation (superadmin only)
	billingAuto := admin.Group("/billing", middleware.RequireAdminRole("superadmin"))
	billingAuto.Post("/generate", adminHandlers.GenerateInvoiceForPartner(dbPool))
	billingAuto.Post("/generate/batch", adminHandlers.GenerateBatchInvoices(dbPool))
	billingAuto.Post("/seed", adminHandlers.SeedSampleInvoices(dbPool))

	// Routes — Partner portal (JWT protected)
	partner := app.Group("/api/partner", middleware.PartnerPortalAuthMiddleware())
	partner.Get("/me", partnerPortalHandlers.Me())
	partner.Get("/me/profile", partnerPortalHandlers.GetProfile(dbPool))
	partner.Patch("/me/profile", partnerPortalHandlers.UpdateProfile(dbPool))
	partner.Patch("/me/password", partnerPortalHandlers.ChangePassword(dbPool))
	partner.Get("/dashboard", partnerPortalHandlers.Dashboard(dbPool))
	partner.Get("/api-keys", partnerPortalHandlers.ListAPIKeys(dbPool))
	partner.Post("/api-keys", partnerPortalHandlers.CreateAPIKey(dbPool))
	partner.Delete("/api-keys/:id", partnerPortalHandlers.RevokeAPIKey(dbPool))
	partner.Get("/usage", partnerPortalHandlers.Usage(dbPool))
	partner.Get("/co2", partnerPortalHandlers.CO2Overview(dbPool))
	partner.Get("/billing", partnerPortalHandlers.Billing(dbPool))

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
