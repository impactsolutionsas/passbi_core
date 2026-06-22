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

type partnerRow struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	Company           *string    `json:"company"`
	Status            string     `json:"status"`
	Tier              string     `json:"tier"`
	RateLimitPerDay   int        `json:"rate_limit_per_day"`
	RateLimitPerMonth int        `json:"rate_limit_per_month"`
	ContactName       *string    `json:"contact_name"`
	ContactPhone      *string    `json:"contact_phone"`
	BillingEmail      *string    `json:"billing_email"`
	PortalEnabled     bool       `json:"portal_enabled"`
	CreatedAt         time.Time  `json:"created_at"`
	LastActiveAt      *time.Time `json:"last_active_at"`
	// aggregates
	TotalKeys   int   `json:"total_keys"`
	CallsMonth  int64 `json:"calls_month"`
}

// ListPartners GET /api/admin/partners?page=1&limit=20&status=active&tier=starter&q=name
func ListPartners(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := max(c.QueryInt("page", 1), 1)
		limit := min(c.QueryInt("limit", 20), 100)
		offset := (page - 1) * limit
		status := c.Query("status")
		tier := c.Query("tier")
		q := strings.TrimSpace(c.Query("q"))

		// Build dynamic WHERE
		conditions := []string{"1=1"}
		args := []any{}
		idx := 1

		if status != "" {
			conditions = append(conditions, fmt.Sprintf("p.status = $%d", idx))
			args = append(args, status)
			idx++
		}
		if tier != "" {
			conditions = append(conditions, fmt.Sprintf("p.tier = $%d", idx))
			args = append(args, tier)
			idx++
		}
		if q != "" {
			conditions = append(conditions, fmt.Sprintf(
				"(p.name ILIKE $%d OR p.email ILIKE $%d OR p.company ILIKE $%d)",
				idx, idx, idx,
			))
			args = append(args, "%"+q+"%")
			idx++
		}

		where := strings.Join(conditions, " AND ")

		monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)

		query := fmt.Sprintf(`
			SELECT
				p.id, p.name, p.email, p.company, p.status, p.tier,
				p.rate_limit_per_day, p.rate_limit_per_month,
				p.contact_name, p.contact_phone, p.billing_email,
				p.portal_enabled, p.created_at, p.last_active_at,
				COUNT(ak.id) AS total_keys,
				COALESCE(SUM(CASE WHEN ul.timestamp >= $%d THEN 1 ELSE 0 END), 0) AS calls_month
			FROM partner p
			LEFT JOIN api_key ak ON ak.partner_id = p.id AND ak.is_active = true
			LEFT JOIN usage_log ul ON ul.partner_id = p.id
			WHERE %s
			GROUP BY p.id
			ORDER BY p.created_at DESC
			LIMIT $%d OFFSET $%d
		`, idx, where, idx+1, idx+2)

		args = append(args, monthStart, limit, offset)

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		partners := []partnerRow{}
		for rows.Next() {
			var p partnerRow
			if err := rows.Scan(
				&p.ID, &p.Name, &p.Email, &p.Company, &p.Status, &p.Tier,
				&p.RateLimitPerDay, &p.RateLimitPerMonth,
				&p.ContactName, &p.ContactPhone, &p.BillingEmail,
				&p.PortalEnabled, &p.CreatedAt, &p.LastActiveAt,
				&p.TotalKeys, &p.CallsMonth,
			); err != nil {
				continue
			}
			partners = append(partners, p)
		}

		// Total count
		countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM partner p WHERE %s`, where)
		countArgs := args[:len(args)-3] // remove monthStart, limit, offset
		var total int
		_ = db.QueryRow(ctx, countQuery, countArgs...).Scan(&total)

		return c.JSON(fiber.Map{
			"data":  partners,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// GetPartner GET /api/admin/partners/:id
func GetPartner(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)

		var p partnerRow
		err := db.QueryRow(ctx, `
			SELECT
				p.id, p.name, p.email, p.company, p.status, p.tier,
				p.rate_limit_per_day, p.rate_limit_per_month,
				p.contact_name, p.contact_phone, p.billing_email,
				p.portal_enabled, p.created_at, p.last_active_at,
				COUNT(ak.id) AS total_keys,
				COALESCE(SUM(CASE WHEN ul.timestamp >= $2 THEN 1 ELSE 0 END), 0) AS calls_month
			FROM partner p
			LEFT JOIN api_key ak ON ak.partner_id = p.id AND ak.is_active = true
			LEFT JOIN usage_log ul ON ul.partner_id = p.id
			WHERE p.id = $1
			GROUP BY p.id
		`, id, monthStart).Scan(
			&p.ID, &p.Name, &p.Email, &p.Company, &p.Status, &p.Tier,
			&p.RateLimitPerDay, &p.RateLimitPerMonth,
			&p.ContactName, &p.ContactPhone, &p.BillingEmail,
			&p.PortalEnabled, &p.CreatedAt, &p.LastActiveAt,
			&p.TotalKeys, &p.CallsMonth,
		)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "partner_not_found"})
		}

		return c.JSON(p)
	}
}

type createPartnerRequest struct {
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Company      *string `json:"company"`
	Tier         string  `json:"tier"`
	ContactName  *string `json:"contact_name"`
	ContactPhone *string `json:"contact_phone"`
	BillingEmail *string `json:"billing_email"`
}

// CreatePartner POST /api/admin/partners
func CreatePartner(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req createPartnerRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Name == "" || req.Email == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name and email are required"})
		}
		if req.Tier == "" {
			req.Tier = "free"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var rateDay, rateMonth, rateSecond int
		_ = db.QueryRow(ctx,
			`SELECT rate_limit_per_second, rate_limit_per_day, rate_limit_per_month FROM tier_config WHERE tier = $1`,
			req.Tier,
		).Scan(&rateSecond, &rateDay, &rateMonth)

		var id string
		err := db.QueryRow(ctx, `
			INSERT INTO partner (name, email, company, tier, status, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, contact_name, contact_phone, billing_email)
			VALUES ($1, $2, $3, $4, 'active', $5, $6, $7, $8, $9, $10)
			RETURNING id
		`, req.Name, req.Email, req.Company, req.Tier, rateSecond, rateDay, rateMonth,
			req.ContactName, req.ContactPhone, req.BillingEmail,
		).Scan(&id)

		if err != nil {
			if strings.Contains(err.Error(), "unique") {
				return c.Status(409).JSON(fiber.Map{"error": "email_already_exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventPartnerCreated,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ActorUA:      c.Get("User-Agent"),
			ResourceType: "partner",
			ResourceID:   id,
			Action:       audit.ActionCreate,
			RequestID:    reqID,
			AfterState: audit.Sanitize(map[string]any{
				"name": req.Name, "email": req.Email,
				"company": req.Company, "tier": req.Tier,
			}),
		})

		return c.Status(201).JSON(fiber.Map{"id": id})
	}
}

type updatePartnerRequest struct {
	Name          *string `json:"name"`
	Company       *string `json:"company"`
	Status        *string `json:"status"`
	Tier          *string `json:"tier"`
	ContactName   *string `json:"contact_name"`
	ContactPhone  *string `json:"contact_phone"`
	BillingEmail  *string `json:"billing_email"`
	PortalEnabled *bool   `json:"portal_enabled"`
}

// UpdatePartner PATCH /api/admin/partners/:id
func UpdatePartner(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req updatePartnerRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Capture before state
		var before struct {
			Name    string  `json:"name"`
			Email   string  `json:"email"`
			Status  string  `json:"status"`
			Tier    string  `json:"tier"`
			Company *string `json:"company"`
		}
		_ = db.QueryRow(ctx,
			`SELECT name, email, status, tier, company FROM partner WHERE id = $1`, id,
		).Scan(&before.Name, &before.Email, &before.Status, &before.Tier, &before.Company)

		sets := []string{}
		args := []any{}
		idx := 1

		if req.Name != nil {
			sets = append(sets, fmt.Sprintf("name = $%d", idx)); args = append(args, *req.Name); idx++
		}
		if req.Company != nil {
			sets = append(sets, fmt.Sprintf("company = $%d", idx)); args = append(args, *req.Company); idx++
		}
		if req.Status != nil {
			sets = append(sets, fmt.Sprintf("status = $%d", idx)); args = append(args, *req.Status); idx++
		}
		if req.ContactName != nil {
			sets = append(sets, fmt.Sprintf("contact_name = $%d", idx)); args = append(args, *req.ContactName); idx++
		}
		if req.ContactPhone != nil {
			sets = append(sets, fmt.Sprintf("contact_phone = $%d", idx)); args = append(args, *req.ContactPhone); idx++
		}
		if req.BillingEmail != nil {
			sets = append(sets, fmt.Sprintf("billing_email = $%d", idx)); args = append(args, *req.BillingEmail); idx++
		}
		if req.PortalEnabled != nil {
			sets = append(sets, fmt.Sprintf("portal_enabled = $%d", idx)); args = append(args, *req.PortalEnabled); idx++
		}
		if req.Tier != nil {
			sets = append(sets, fmt.Sprintf("tier = $%d", idx)); args = append(args, *req.Tier); idx++
			var rateDay, rateMonth, rateSecond int
			_ = db.QueryRow(ctx,
				`SELECT rate_limit_per_second, rate_limit_per_day, rate_limit_per_month FROM tier_config WHERE tier = $1`,
				*req.Tier,
			).Scan(&rateSecond, &rateDay, &rateMonth)
			sets = append(sets,
				fmt.Sprintf("rate_limit_per_second = $%d", idx),
				fmt.Sprintf("rate_limit_per_day = $%d", idx+1),
				fmt.Sprintf("rate_limit_per_month = $%d", idx+2),
			)
			args = append(args, rateSecond, rateDay, rateMonth)
			idx += 3
		}

		if len(sets) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "no fields to update"})
		}

		args = append(args, id)
		_, err := db.Exec(ctx,
			fmt.Sprintf("UPDATE partner SET %s WHERE id = $%d", strings.Join(sets, ", "), idx),
			args...,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventPartnerUpdated,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "partner",
			ResourceID:   id,
			Action:       audit.ActionUpdate,
			RequestID:    reqID,
			BeforeState:  before,
			AfterState:   req,
		})

		return c.SendStatus(204)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
