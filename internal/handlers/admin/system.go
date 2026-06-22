package admin

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/cache"
)

type systemHealth struct {
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	DB        dbHealth       `json:"db"`
	Redis     redisHealth    `json:"redis"`
	Runtime   runtimeHealth  `json:"runtime"`
}

type dbHealth struct {
	OK           bool   `json:"ok"`
	Latency      string `json:"latency_ms"`
	OpenConns    int32  `json:"open_conns"`
	IdleConns    int32  `json:"idle_conns"`
	TotalStops   int    `json:"total_stops"`
	TotalRoutes  int    `json:"total_routes"`
	TotalEdges   int    `json:"total_edges"`
}

type redisHealth struct {
	OK        bool           `json:"ok"`
	Latency   string         `json:"latency_ms"`
	Stats     map[string]any `json:"stats"`
}

type runtimeHealth struct {
	GoVersion  string `json:"go_version"`
	Goroutines int    `json:"goroutines"`
	MemAllocMB uint64 `json:"mem_alloc_mb"`
}

// SystemHealth GET /api/admin/system/health
func SystemHealth(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		health := systemHealth{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		// DB health
		dbStart := time.Now()
		dbErr := db.Ping(ctx)
		health.DB.Latency = fmt.Sprintf("%d", time.Since(dbStart).Milliseconds())
		health.DB.OK = dbErr == nil

		if health.DB.OK {
			stats := db.Stat()
			health.DB.OpenConns = stats.AcquiredConns()
			health.DB.IdleConns = stats.IdleConns()

			_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM stop`).Scan(&health.DB.TotalStops)
			_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM route`).Scan(&health.DB.TotalRoutes)
			_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM edge`).Scan(&health.DB.TotalEdges)
		} else {
			health.Status = "degraded"
		}

		// Redis health
		redisStart := time.Now()
		redisErr := cache.HealthCheck(ctx)
		health.Redis.Latency = fmt.Sprintf("%d", time.Since(redisStart).Milliseconds())
		health.Redis.OK = redisErr == nil

		if health.Redis.OK {
			health.Redis.Stats, _ = cache.Stats(ctx)
		} else {
			health.Status = "degraded"
		}

		// Runtime
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		health.Runtime = runtimeHealth{
			GoVersion:  runtime.Version(),
			Goroutines: runtime.NumGoroutine(),
			MemAllocMB: mem.Alloc / 1024 / 1024,
		}

		status := 200
		if health.Status != "ok" {
			status = 503
		}

		return c.Status(status).JSON(health)
	}
}
