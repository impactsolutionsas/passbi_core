package partner_portal

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

type usageDailyStat struct {
	Date      string  `json:"date"`
	Calls     int64   `json:"calls"`
	Errors    int64   `json:"errors"`
	CacheHits int64   `json:"cache_hits"`
	AvgMs     float64 `json:"avg_ms"`
}

type recentLog struct {
	Endpoint     string  `json:"endpoint"`
	Method       string  `json:"method"`
	Status       int     `json:"status"`
	ResponseMs   int     `json:"response_ms"`
	CacheHit     bool    `json:"cache_hit"`
	Timestamp    string  `json:"timestamp"`
}

type usageResponse struct {
	Daily       []usageDailyStat `json:"daily"`
	Recent      []recentLog      `json:"recent"`
	SuccessRate float64          `json:"success_rate"`
	AvgLatency  float64          `json:"avg_latency_ms"`
	CacheRate   float64          `json:"cache_hit_rate"`
}

// Usage GET /api/partner/usage?days=30
func Usage(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		days := c.QueryInt("days", 30)
		if days > 90 {
			days = 90
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		since := time.Now().AddDate(0, 0, -days)

		// Daily stats
		rows, err := db.Query(ctx, `
			SELECT
				DATE(timestamp)::text,
				COUNT(*),
				COUNT(*) FILTER (WHERE response_status >= 400),
				COUNT(*) FILTER (WHERE cache_hit = true),
				ROUND(AVG(response_time_ms)::numeric, 1)
			FROM usage_log
			WHERE partner_id = $1 AND timestamp >= $2
			GROUP BY DATE(timestamp)
			ORDER BY DATE(timestamp)
		`, p.PartnerID, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		daily := []usageDailyStat{}
		for rows.Next() {
			var d usageDailyStat
			_ = rows.Scan(&d.Date, &d.Calls, &d.Errors, &d.CacheHits, &d.AvgMs)
			daily = append(daily, d)
		}
		rows.Close()

		// Recent logs (last 50)
		recentRows, err := db.Query(ctx, `
			SELECT endpoint, method, response_status, response_time_ms, cache_hit,
			       TO_CHAR(timestamp, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM usage_log
			WHERE partner_id = $1
			ORDER BY timestamp DESC
			LIMIT 50
		`, p.PartnerID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer recentRows.Close()

		recent := []recentLog{}
		for recentRows.Next() {
			var l recentLog
			_ = recentRows.Scan(&l.Endpoint, &l.Method, &l.Status, &l.ResponseMs, &l.CacheHit, &l.Timestamp)
			recent = append(recent, l)
		}
		recentRows.Close()

		// Aggregate stats over period
		var successRate, avgLatency, cacheRate float64
		_ = db.QueryRow(ctx, `
			SELECT
				COALESCE(ROUND(100.0 * COUNT(*) FILTER (WHERE response_status < 400) / NULLIF(COUNT(*),0), 1), 100),
				COALESCE(ROUND(AVG(response_time_ms)::numeric, 1), 0),
				COALESCE(ROUND(100.0 * COUNT(*) FILTER (WHERE cache_hit) / NULLIF(COUNT(*),0), 1), 0)
			FROM usage_log
			WHERE partner_id = $1 AND timestamp >= $2
		`, p.PartnerID, since).Scan(&successRate, &avgLatency, &cacheRate)

		return c.JSON(usageResponse{
			Daily:       daily,
			Recent:      recent,
			SuccessRate: successRate,
			AvgLatency:  avgLatency,
			CacheRate:   cacheRate,
		})
	}
}
