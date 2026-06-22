package admin

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type kpiResponse struct {
	TotalPartners  int     `json:"total_partners"`
	ActivePartners int     `json:"active_partners"`
	CallsToday     int64   `json:"calls_today"`
	CallsMonth     int64   `json:"calls_month"`
	P95LatencyMs   float64 `json:"p95_latency_ms"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	ErrorRate      float64 `json:"error_rate"`
}

func DashboardKPIs(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var kpis kpiResponse

		// Partner counts
		_ = db.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'active') FROM partner`).
			Scan(&kpis.TotalPartners, &kpis.ActivePartners)

		// Calls today
		today := time.Now().Truncate(24 * time.Hour)
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM usage_log WHERE timestamp >= $1`, today).
			Scan(&kpis.CallsToday)

		// Calls this month
		monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM usage_log WHERE timestamp >= $1`, monthStart).
			Scan(&kpis.CallsMonth)

		// P95 latency (last 24h)
		_ = db.QueryRow(ctx, `
			SELECT COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time_ms), 0)
			FROM usage_log
			WHERE timestamp >= NOW() - INTERVAL '24 hours'`).
			Scan(&kpis.P95LatencyMs)

		// Cache hit rate (last 24h)
		_ = db.QueryRow(ctx, `
			SELECT COALESCE(
				ROUND(100.0 * COUNT(*) FILTER (WHERE cache_hit = true) / NULLIF(COUNT(*), 0), 1),
				0
			)
			FROM usage_log
			WHERE timestamp >= NOW() - INTERVAL '24 hours'`).
			Scan(&kpis.CacheHitRate)

		// Error rate (last 24h) — 5xx
		_ = db.QueryRow(ctx, `
			SELECT COALESCE(
				ROUND(100.0 * COUNT(*) FILTER (WHERE response_status >= 500) / NULLIF(COUNT(*), 0), 1),
				0
			)
			FROM usage_log
			WHERE timestamp >= NOW() - INTERVAL '24 hours'`).
			Scan(&kpis.ErrorRate)

		return c.JSON(kpis)
	}
}
