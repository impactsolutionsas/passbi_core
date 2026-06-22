package admin

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dailyStat struct {
	Date      string  `json:"date"`
	Calls     int64   `json:"calls"`
	Errors    int64   `json:"errors"`
	CacheHits int64   `json:"cache_hits"`
	AvgMs     float64 `json:"avg_ms"`
}

type endpointStat struct {
	Endpoint string  `json:"endpoint"`
	Calls    int64   `json:"calls"`
	AvgMs    float64 `json:"avg_ms"`
	ErrorPct float64 `json:"error_pct"`
}

type partnerStat struct {
	PartnerID   string  `json:"partner_id"`
	PartnerName string  `json:"partner_name"`
	Calls       int64   `json:"calls"`
	ErrorPct    float64 `json:"error_pct"`
	Tier        string  `json:"tier"`
}

type analyticsOverview struct {
	Daily     []dailyStat    `json:"daily"`
	Endpoints []endpointStat `json:"endpoints"`
	Partners  []partnerStat  `json:"top_partners"`
}

// AnalyticsOverview GET /api/admin/analytics/overview?days=30
func AnalyticsOverview(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		days := c.QueryInt("days", 30)
		if days > 90 {
			days = 90
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		since := time.Now().AddDate(0, 0, -days)

		// Daily stats
		dailyRows, err := db.Query(ctx, `
			SELECT
				DATE(timestamp)::text AS date,
				COUNT(*) AS calls,
				COUNT(*) FILTER (WHERE response_status >= 500) AS errors,
				COUNT(*) FILTER (WHERE cache_hit = true) AS cache_hits,
				ROUND(AVG(response_time_ms)::numeric, 1) AS avg_ms
			FROM usage_log
			WHERE timestamp >= $1
			GROUP BY DATE(timestamp)
			ORDER BY DATE(timestamp)
		`, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer dailyRows.Close()

		daily := []dailyStat{}
		for dailyRows.Next() {
			var d dailyStat
			_ = dailyRows.Scan(&d.Date, &d.Calls, &d.Errors, &d.CacheHits, &d.AvgMs)
			daily = append(daily, d)
		}

		// Top endpoints
		epRows, err := db.Query(ctx, `
			SELECT
				endpoint,
				COUNT(*) AS calls,
				ROUND(AVG(response_time_ms)::numeric, 1) AS avg_ms,
				ROUND(100.0 * COUNT(*) FILTER (WHERE response_status >= 400) / NULLIF(COUNT(*), 0), 1) AS error_pct
			FROM usage_log
			WHERE timestamp >= $1
			GROUP BY endpoint
			ORDER BY calls DESC
			LIMIT 10
		`, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer epRows.Close()

		endpoints := []endpointStat{}
		for epRows.Next() {
			var e endpointStat
			_ = epRows.Scan(&e.Endpoint, &e.Calls, &e.AvgMs, &e.ErrorPct)
			endpoints = append(endpoints, e)
		}

		// Top partners
		pRows, err := db.Query(ctx, `
			SELECT
				ul.partner_id,
				p.name,
				COUNT(*) AS calls,
				ROUND(100.0 * COUNT(*) FILTER (WHERE ul.response_status >= 400) / NULLIF(COUNT(*), 0), 1) AS error_pct,
				p.tier
			FROM usage_log ul
			JOIN partner p ON p.id = ul.partner_id
			WHERE ul.timestamp >= $1
			GROUP BY ul.partner_id, p.name, p.tier
			ORDER BY calls DESC
			LIMIT 10
		`, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer pRows.Close()

		partners := []partnerStat{}
		for pRows.Next() {
			var p partnerStat
			_ = pRows.Scan(&p.PartnerID, &p.PartnerName, &p.Calls, &p.ErrorPct, &p.Tier)
			partners = append(partners, p)
		}

		return c.JSON(analyticsOverview{
			Daily:     daily,
			Endpoints: endpoints,
			Partners:  partners,
		})
	}
}
