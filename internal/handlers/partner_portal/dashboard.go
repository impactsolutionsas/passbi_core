package partner_portal

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

type dashboardResponse struct {
	CallsToday      int64   `json:"calls_today"`
	CallsThisMonth  int64   `json:"calls_this_month"`
	QuotaDay        int     `json:"quota_day"`
	QuotaMonth      int     `json:"quota_month"`
	ActiveKeys      int     `json:"active_keys"`
	SuccessRate     float64 `json:"success_rate"`
}

func Dashboard(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p, ok := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var resp dashboardResponse

		// Calls today
		today := time.Now().Truncate(24 * time.Hour)
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM usage_log WHERE partner_id = $1 AND timestamp >= $2`,
			p.PartnerID, today).Scan(&resp.CallsToday)

		// Calls this month
		monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM usage_log WHERE partner_id = $1 AND timestamp >= $2`,
			p.PartnerID, monthStart).Scan(&resp.CallsThisMonth)

		// Rate limits from partner table
		_ = db.QueryRow(ctx,
			`SELECT rate_limit_per_day, rate_limit_per_month FROM partner WHERE id = $1`,
			p.PartnerID).Scan(&resp.QuotaDay, &resp.QuotaMonth)

		// Active API keys
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM api_key WHERE partner_id = $1 AND is_active = true`, p.PartnerID).
			Scan(&resp.ActiveKeys)

		// Success rate (last 30 days)
		_ = db.QueryRow(ctx, `
			SELECT COALESCE(
				ROUND(100.0 * COUNT(*) FILTER (WHERE response_status < 400) / NULLIF(COUNT(*), 0), 1),
				100
			)
			FROM usage_log
			WHERE partner_id = $1 AND timestamp >= NOW() - INTERVAL '30 days'`,
			p.PartnerID).Scan(&resp.SuccessRate)

		return c.JSON(resp)
	}
}
