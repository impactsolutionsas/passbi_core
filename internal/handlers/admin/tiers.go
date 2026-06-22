package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/audit"
	"github.com/passbi/passbi_core/internal/middleware"
)

type tierConfig struct {
	Tier              string         `json:"tier"`
	PriceCents        int            `json:"price_cents"`
	RateLimitPerSec   int            `json:"rate_limit_per_second"`
	RateLimitPerDay   int            `json:"rate_limit_per_day"`
	RateLimitPerMonth int            `json:"rate_limit_per_month"`
	Features          map[string]any `json:"features"`
}

// ListTiers GET /api/admin/tiers
func ListTiers(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT tier, price_cents, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, features
			FROM tier_config
			ORDER BY price_cents
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		tiers := []tierConfig{}
		for rows.Next() {
			var t tierConfig
			if err := rows.Scan(
				&t.Tier, &t.PriceCents, &t.RateLimitPerSec,
				&t.RateLimitPerDay, &t.RateLimitPerMonth, &t.Features,
			); err != nil {
				continue
			}
			tiers = append(tiers, t)
		}

		return c.JSON(tiers)
	}
}

type updateTierRequest struct {
	PriceCents        *int            `json:"price_cents"`
	RateLimitPerSec   *int            `json:"rate_limit_per_second"`
	RateLimitPerDay   *int            `json:"rate_limit_per_day"`
	RateLimitPerMonth *int            `json:"rate_limit_per_month"`
	Features          map[string]any  `json:"features"`
}

// UpdateTier PATCH /api/admin/tiers/:tier
func UpdateTier(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tier := c.Params("tier")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req updateTierRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Capture before state
		var before tierConfig
		_ = db.QueryRow(ctx,
			`SELECT tier, price_cents, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, features FROM tier_config WHERE tier = $1`,
			tier,
		).Scan(&before.Tier, &before.PriceCents, &before.RateLimitPerSec, &before.RateLimitPerDay, &before.RateLimitPerMonth, &before.Features)

		sets := []string{}
		args := []any{}
		idx := 1

		if req.PriceCents != nil {
			sets = append(sets, fmt.Sprintf("price_cents = $%d", idx)); args = append(args, *req.PriceCents); idx++
		}
		if req.RateLimitPerSec != nil {
			sets = append(sets, fmt.Sprintf("rate_limit_per_second = $%d", idx)); args = append(args, *req.RateLimitPerSec); idx++
		}
		if req.RateLimitPerDay != nil {
			sets = append(sets, fmt.Sprintf("rate_limit_per_day = $%d", idx)); args = append(args, *req.RateLimitPerDay); idx++
		}
		if req.RateLimitPerMonth != nil {
			sets = append(sets, fmt.Sprintf("rate_limit_per_month = $%d", idx)); args = append(args, *req.RateLimitPerMonth); idx++
		}
		if req.Features != nil {
			sets = append(sets, fmt.Sprintf("features = $%d", idx)); args = append(args, req.Features); idx++
		}

		if len(sets) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "no fields to update"})
		}

		args = append(args, tier)
		_, err := db.Exec(ctx,
			fmt.Sprintf("UPDATE tier_config SET %s WHERE tier = $%d", strings.Join(sets, ", "), idx),
			args...,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventTierUpdated,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "tier_config",
			ResourceID:   tier,
			Action:       audit.ActionUpdate,
			RequestID:    reqID,
			BeforeState:  before,
			AfterState:   req,
		})

		return c.SendStatus(204)
	}
}
