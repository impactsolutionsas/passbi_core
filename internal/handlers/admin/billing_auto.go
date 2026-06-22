package admin

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

// ─── Invoice line item used internally during generation ─────────────────────

type autoItem struct {
	Section     string
	Description string
	Quantity    int64
	UnitPrice   int64 // smallest currency unit (XOF = FCFA, USD = cents)
	Amount      int64
}

// ─── XOF Tier pricing (from tier_config.price_xof) ───────────────────────────

type tierInfo struct {
	PriceXOF       int64
	RateLimitMonth int
}

func loadTierInfo(ctx context.Context, db *pgxpool.Pool, tier string) (tierInfo, error) {
	var t tierInfo
	err := db.QueryRow(ctx,
		`SELECT COALESCE(price_xof, 0), rate_limit_per_month FROM tier_config WHERE tier = $1`,
		tier,
	).Scan(&t.PriceXOF, &t.RateLimitMonth)
	return t, err
}

// ─── Core: build invoice items for a partner+period ──────────────────────────

// buildInvoiceItems constructs Supabase-style line items.
// If simulatedUsage is non-nil it is used instead of querying usage_log (for seeds).
func buildInvoiceItems(
	ctx context.Context,
	db *pgxpool.Pool,
	partnerID string,
	tierName string,
	ti tierInfo,
	periodStart, periodEnd time.Time,
	simulatedUsage map[string]int64, // endpoint → calls; nil = query DB
) ([]autoItem, int64) {

	var items []autoItem
	var subtotal int64

	// ── Section 1 : Abonnement ───────────────────────────────────────────────
	if ti.PriceXOF > 0 {
		label := fmt.Sprintf("%s Plan (%s – %s)",
			strings.Title(tierName),
			periodStart.Format("2 Jan"),
			periodEnd.AddDate(0, 0, -1).Format("2 Jan 2006"),
		)
		items = append(items, autoItem{
			Section:     "Abonnement",
			Description: label,
			Quantity:    1,
			UnitPrice:   ti.PriceXOF,
			Amount:      ti.PriceXOF,
		})
		subtotal += ti.PriceXOF
	}

	// ── Section 2 : Appels API ───────────────────────────────────────────────
	usage := simulatedUsage
	if usage == nil {
		usage = map[string]int64{}
		rows, err := db.Query(ctx, `
			SELECT endpoint, COUNT(*) AS calls
			FROM usage_log
			WHERE partner_id = $1 AND timestamp >= $2 AND timestamp < $3
			GROUP BY endpoint
			ORDER BY calls DESC
		`, partnerID, periodStart, periodEnd)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ep string
				var n int64
				_ = rows.Scan(&ep, &n)
				usage[ep] = n
			}
		}
	}

	orderedEndpoints := []string{
		"/v2/route-search",
		"/v2/stops/nearby",
		"/v2/stops/search",
		"/v2/stops/:id/departures",
		"/v2/routes/list",
		"/v2/routes/:id/schedule",
		"/v2/routes/:id/trips",
	}
	// Ensure any extra endpoints from usage are appended
	seen := map[string]bool{}
	for _, ep := range orderedEndpoints {
		seen[ep] = true
	}
	for ep := range usage {
		if !seen[ep] {
			orderedEndpoints = append(orderedEndpoints, ep)
		}
	}

	var totalCalls int64
	for _, ep := range orderedEndpoints {
		n, ok := usage[ep]
		if !ok || n == 0 {
			continue
		}
		totalCalls += n
		items = append(items, autoItem{
			Section:     "Appels API",
			Description: ep,
			Quantity:    n,
			UnitPrice:   0,
			Amount:      0,
		})
	}

	// Quota discount line (shows included quota)
	if len(items) > 0 && ti.RateLimitMonth > 0 {
		included := ti.RateLimitMonth
		if totalCalls < int64(included) {
			included = int(totalCalls)
		}
		items = append(items, autoItem{
			Section:     "Appels API",
			Description: fmt.Sprintf("Quota inclus (-%s appels)", formatInt(int64(included))),
			Quantity:    0,
			UnitPrice:   0,
			Amount:      0,
		})
	}

	// ── Section 3 : Dépassement quota ───────────────────────────────────────
	if ti.RateLimitMonth > 0 && totalCalls > int64(ti.RateLimitMonth) {
		overage := totalCalls - int64(ti.RateLimitMonth)
		// 1 XOF per 100 extra calls (0.01 XOF/call)
		overagePerCall := int64(1)
		overageAmount := overage * overagePerCall
		items = append(items, autoItem{
			Section:     "Dépassement quota",
			Description: fmt.Sprintf("Appels supplémentaires (%s inclus)", formatInt(int64(ti.RateLimitMonth))),
			Quantity:    overage,
			UnitPrice:   overagePerCall,
			Amount:      overageAmount,
		})
		subtotal += overageAmount
	}

	return items, subtotal
}

func formatInt(n int64) string {
	// Simple thousand-separator formatter (no external deps)
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ' ')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// ─── Persist a generated invoice ─────────────────────────────────────────────

func persistAutoInvoice(
	ctx context.Context,
	db *pgxpool.Pool,
	partnerID string,
	createdByID string,
	currency string,
	periodStart time.Time,
	items []autoItem,
	subtotal int64,
	status string, // "draft" | "sent" | "paid"
	paidAt *time.Time,
) (string, string, error) {

	invoiceNum, err := nextInvoiceNumber(ctx, db)
	if err != nil {
		return "", "", err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	// due date = last day of billing month
	dueDate := periodStart.AddDate(0, 1, -1)

	var invoiceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO invoice
			(invoice_number, partner_id, created_by, status, currency,
			 subtotal, tax_rate, tax_amount, total, due_date, paid_at,
			 sent_at, notes)
		VALUES ($1,$2,$3,$4,$5,$6,0,0,$6,$7,$8,$9,
			'Facture générée automatiquement — usage du mois')
		RETURNING id
	`,
		invoiceNum, partnerID, createdByID, status, currency,
		subtotal, dueDate,
		paidAt,
		func() *time.Time {
			if status == "sent" || status == "paid" {
				t := periodStart.Add(24 * time.Hour)
				return &t
			}
			return nil
		}(),
	).Scan(&invoiceID)
	if err != nil {
		return "", "", err
	}

	for pos, it := range items {
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_item
				(invoice_id, position, section, description, quantity, unit_price, amount)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, invoiceID, pos+1, it.Section, it.Description, it.Quantity, it.UnitPrice, it.Amount)
		if err != nil {
			return "", "", err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return invoiceID, invoiceNum, nil
}

// ─── Handler: generate invoice for one partner + month ───────────────────────

type generateInvoiceRequest struct {
	PartnerID string `json:"partner_id"`
	Year      int    `json:"year"`
	Month     int    `json:"month"` // 1-12
	Currency  string `json:"currency"`
}

// GenerateInvoiceForPartner POST /api/admin/billing/generate
func GenerateInvoiceForPartner(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)

		var req generateInvoiceRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.PartnerID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "partner_id required"})
		}
		if req.Year == 0 {
			req.Year = time.Now().Year()
		}
		if req.Month == 0 {
			req.Month = int(time.Now().Month())
		}
		if req.Currency == "" {
			req.Currency = "XOF"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Load partner + tier
		var partnerName, tier string
		if err := db.QueryRow(ctx,
			`SELECT name, tier FROM partner WHERE id = $1 AND status = 'active'`,
			req.PartnerID,
		).Scan(&partnerName, &tier); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "partner_not_found"})
		}

		ti, err := loadTierInfo(ctx, db, tier)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "tier_not_found"})
		}

		periodStart := time.Date(req.Year, time.Month(req.Month), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, 0)

		items, subtotal := buildInvoiceItems(ctx, db, req.PartnerID, tier, ti, periodStart, periodEnd, nil)
		if len(items) == 0 {
			return c.Status(422).JSON(fiber.Map{"error": "no_billable_items", "hint": "free tier with no usage"})
		}

		invoiceID, invoiceNum, err := persistAutoInvoice(
			ctx, db, req.PartnerID, actor.AdminID,
			req.Currency, periodStart, items, subtotal, "draft", nil,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(201).JSON(fiber.Map{
			"id":             invoiceID,
			"invoice_number": invoiceNum,
			"partner":        partnerName,
			"total":          subtotal,
			"currency":       req.Currency,
			"items":          len(items),
		})
	}
}

// ─── Handler: generate for ALL paid partners (current month) ─────────────────

// GenerateBatchInvoices POST /api/admin/billing/generate/batch
func GenerateBatchInvoices(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, 0)

		rows, err := db.Query(ctx, `
			SELECT id, name, tier FROM partner
			WHERE status = 'active' AND tier != 'free'
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		type result struct {
			PartnerID   string `json:"partner_id"`
			Partner     string `json:"partner"`
			InvoiceID   string `json:"invoice_id"`
			InvoiceNum  string `json:"invoice_number"`
			Total       int64  `json:"total"`
			Error       string `json:"error,omitempty"`
		}
		var results []result

		for rows.Next() {
			var pid, pname, tier string
			_ = rows.Scan(&pid, &pname, &tier)

			ti, err := loadTierInfo(ctx, db, tier)
			if err != nil {
				results = append(results, result{PartnerID: pid, Partner: pname, Error: "tier_not_found"})
				continue
			}

			items, subtotal := buildInvoiceItems(ctx, db, pid, tier, ti, periodStart, periodEnd, nil)
			if len(items) == 0 {
				results = append(results, result{PartnerID: pid, Partner: pname, Error: "no_billable_items"})
				continue
			}

			invID, invNum, err := persistAutoInvoice(
				ctx, db, pid, actor.AdminID, "XOF", periodStart, items, subtotal, "draft", nil,
			)
			if err != nil {
				results = append(results, result{PartnerID: pid, Partner: pname, Error: err.Error()})
				continue
			}

			results = append(results, result{
				PartnerID: pid, Partner: pname,
				InvoiceID: invID, InvoiceNum: invNum, Total: subtotal,
			})
		}

		return c.JSON(fiber.Map{
			"generated": len(results),
			"results":   results,
		})
	}
}

// ─── Handler: seed sample invoices (realistic demo data) ─────────────────────

// SeedSampleInvoices POST /api/admin/billing/seed
// Generates 3 months of demo invoices for all active paid partners,
// with realistic simulated usage per tier — for display/testing purposes.
func SeedSampleInvoices(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Load all active paid partners
		rows, err := db.Query(ctx, `
			SELECT id, name, tier FROM partner
			WHERE status = 'active' AND tier != 'free'
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		type partnerRecord struct{ id, name, tier string }
		var partners []partnerRecord
		for rows.Next() {
			var p partnerRecord
			_ = rows.Scan(&p.id, &p.name, &p.tier)
			partners = append(partners, p)
		}
		rows.Close()

		if len(partners) == 0 {
			return c.Status(422).JSON(fiber.Map{
				"error": "no_paid_partners",
				"hint":  "Create partners with starter/business/enterprise tier first",
			})
		}

		now := time.Now()
		rng := rand.New(rand.NewSource(42))

		type seedResult struct {
			InvoiceNum string `json:"invoice_number"`
			Partner    string `json:"partner"`
			Tier       string `json:"tier"`
			Total      int64  `json:"total"`
			Status     string `json:"status"`
			Month      string `json:"month"`
		}
		var generated []seedResult

		for _, p := range partners {
			ti, err := loadTierInfo(ctx, db, p.tier)
			if err != nil {
				continue
			}

			// Generate last 3 months
			for monthOffset := -2; monthOffset <= 0; monthOffset++ {
				t := now.AddDate(0, monthOffset, 0)
				periodStart := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
				periodEnd := periodStart.AddDate(0, 1, 0)

				// Simulate realistic usage based on tier
				usage := simulateUsageForTier(p.tier, ti.RateLimitMonth, rng)

				items, subtotal := buildInvoiceItems(
					ctx, db, p.id, p.tier, ti,
					periodStart, periodEnd,
					usage,
				)
				if len(items) == 0 {
					continue
				}

				// Vary status: oldest = paid, middle = sent, current = draft
				status := "draft"
				var paidAt *time.Time
				switch monthOffset {
				case -2:
					status = "paid"
					t := periodStart.AddDate(0, 0, 5)
					paidAt = &t
				case -1:
					status = "sent"
				}

				invID, invNum, err := persistAutoInvoice(
					ctx, db, p.id, actor.AdminID,
					"XOF", periodStart, items, subtotal,
					status, paidAt,
				)
				if err != nil {
					continue
				}

				// If paid, also create payment record
				if status == "paid" && paidAt != nil {
					_, _ = db.Exec(ctx, `
						INSERT INTO payment
							(invoice_id, created_by, amount, currency, method, reference, paid_at)
						VALUES ($1,$2,$3,'XOF','bank_transfer',$4,$5)
					`, invID, actor.AdminID, subtotal,
						fmt.Sprintf("VIR-%s", invNum),
						*paidAt,
					)
				}

				generated = append(generated, seedResult{
					InvoiceNum: invNum,
					Partner:    p.name,
					Tier:       p.tier,
					Total:      subtotal,
					Status:     status,
					Month:      periodStart.Format("2006-01"),
				})
			}
		}

		return c.JSON(fiber.Map{
			"seeded":   len(generated),
			"invoices": generated,
		})
	}
}

// simulateUsageForTier generates realistic API call distribution for a tier.
func simulateUsageForTier(tier string, quotaMonth int, rng *rand.Rand) map[string]int64 {
	// Usage targets: free 60%, starter 70%, business 55%, enterprise variable
	targets := map[string]float64{
		"free":       0.60,
		"starter":    0.72,
		"business":   0.55,
		"enterprise": 0.40,
	}
	pct := targets[tier]
	if pct == 0 {
		pct = 0.5
	}
	// Add ±15% noise
	noise := (rng.Float64()*0.3 - 0.15)
	pct += noise
	if pct < 0.05 {
		pct = 0.05
	}

	base := quotaMonth
	if base <= 0 {
		base = 50000 // enterprise fallback
	}
	total := int64(float64(base) * pct)

	// Distribute across endpoints (realistic weights from transit app)
	weights := map[string]float64{
		"/v2/route-search":          0.45,
		"/v2/stops/nearby":          0.20,
		"/v2/stops/search":          0.12,
		"/v2/stops/:id/departures":  0.11,
		"/v2/routes/list":           0.07,
		"/v2/routes/:id/schedule":   0.03,
		"/v2/routes/:id/trips":      0.02,
	}

	usage := map[string]int64{}
	var allocated int64
	endpoints := []string{
		"/v2/route-search",
		"/v2/stops/nearby",
		"/v2/stops/search",
		"/v2/stops/:id/departures",
		"/v2/routes/list",
		"/v2/routes/:id/schedule",
		"/v2/routes/:id/trips",
	}
	for i, ep := range endpoints {
		w := weights[ep]
		// Last endpoint gets the remainder
		var n int64
		if i == len(endpoints)-1 {
			n = total - allocated
		} else {
			n = int64(float64(total) * w)
			// Add small noise per endpoint
			n += int64(float64(n) * (rng.Float64()*0.1 - 0.05))
		}
		if n < 0 {
			n = 0
		}
		if n > 0 {
			usage[ep] = n
		}
		allocated += n
	}
	return usage
}
