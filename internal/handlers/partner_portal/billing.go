package partner_portal

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

type billingResponse struct {
	Tier              string         `json:"tier"`
	PriceCents        int            `json:"price_cents"`
	Features          map[string]any `json:"features"`
	RateLimitPerSec   int            `json:"rate_limit_per_second"`
	RateLimitPerDay   int            `json:"rate_limit_per_day"`
	RateLimitPerMonth int            `json:"rate_limit_per_month"`
	CurrentMonth      monthUsage     `json:"current_month"`
	History           []monthUsage   `json:"history"`
}

type monthUsage struct {
	Month       string  `json:"month"`
	Calls       int64   `json:"calls"`
	QuotaMonth  int     `json:"quota_month"`
	UsagePct    float64 `json:"usage_pct"`
}

// Billing GET /api/partner/billing
func Billing(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		var resp billingResponse

		// Partner + tier config
		_ = db.QueryRow(ctx, `
			SELECT
				p.tier, tc.price_cents, tc.features,
				p.rate_limit_per_second, p.rate_limit_per_day, p.rate_limit_per_month
			FROM partner p
			JOIN tier_config tc ON tc.tier = p.tier
			WHERE p.id = $1
		`, p.PartnerID).Scan(
			&resp.Tier, &resp.PriceCents, &resp.Features,
			&resp.RateLimitPerSec, &resp.RateLimitPerDay, &resp.RateLimitPerMonth,
		)

		// Current month usage
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		var currentCalls int64
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM usage_log WHERE partner_id = $1 AND timestamp >= $2`,
			p.PartnerID, monthStart,
		).Scan(&currentCalls)

		usagePct := 0.0
		if resp.RateLimitPerMonth > 0 {
			usagePct = float64(currentCalls) / float64(resp.RateLimitPerMonth) * 100
		}
		resp.CurrentMonth = monthUsage{
			Month:      now.Format("2006-01"),
			Calls:      currentCalls,
			QuotaMonth: resp.RateLimitPerMonth,
			UsagePct:   usagePct,
		}

		// Last 6 months history
		rows, err := db.Query(ctx, `
			SELECT
				TO_CHAR(DATE_TRUNC('month', timestamp), 'YYYY-MM') AS month,
				COUNT(*) AS calls
			FROM usage_log
			WHERE partner_id = $1
			  AND timestamp >= NOW() - INTERVAL '6 months'
			  AND timestamp < $2
			GROUP BY DATE_TRUNC('month', timestamp)
			ORDER BY month DESC
		`, p.PartnerID, monthStart)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m monthUsage
				_ = rows.Scan(&m.Month, &m.Calls)
				m.QuotaMonth = resp.RateLimitPerMonth
				if resp.RateLimitPerMonth > 0 {
					m.UsagePct = float64(m.Calls) / float64(resp.RateLimitPerMonth) * 100
				}
				resp.History = append(resp.History, m)
			}
		}
		if resp.History == nil {
			resp.History = []monthUsage{}
		}

		return c.JSON(resp)
	}
}
