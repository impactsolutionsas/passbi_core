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

// ─── Helpers ────────────────────────────────────────────────────────────────

func nextInvoiceNumber(ctx context.Context, db *pgxpool.Pool) (string, error) {
	var seq int64
	err := db.QueryRow(ctx, `SELECT nextval('invoice_seq')`).Scan(&seq)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("INV-%d-%04d", time.Now().Year(), seq), nil
}

func nextDevisNumber(ctx context.Context, db *pgxpool.Pool) (string, error) {
	var seq int64
	err := db.QueryRow(ctx, `SELECT nextval('devis_seq')`).Scan(&seq)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DEV-%d-%04d", time.Now().Year(), seq), nil
}

// ─── Revenue Overview ────────────────────────────────────────────────────────

// BillingRevenue GET /api/admin/billing/revenue
// Accessible to superadmin + accountant.
func BillingRevenue(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

		type currencyStats struct {
			Currency     string `json:"currency"`
			InvoicedMTD  int64  `json:"invoiced_mtd"`
			InvoicedYTD  int64  `json:"invoiced_ytd"`
			CollectedMTD int64  `json:"collected_mtd"`
			CollectedYTD int64  `json:"collected_ytd"`
			Overdue      int64  `json:"overdue"`
		}

		// Per-currency invoiced amounts
		rows, err := db.Query(ctx, `
			SELECT
				currency,
				COALESCE(SUM(CASE WHEN created_at >= $1 AND status != 'cancelled' THEN total ELSE 0 END), 0) AS invoiced_mtd,
				COALESCE(SUM(CASE WHEN created_at >= $2 AND status != 'cancelled' THEN total ELSE 0 END), 0) AS invoiced_ytd,
				COALESCE(SUM(CASE WHEN status IN ('overdue') THEN total ELSE 0 END), 0) AS overdue
			FROM invoice
			GROUP BY currency
		`, monthStart, yearStart)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		statsMap := map[string]*currencyStats{}
		for rows.Next() {
			var s currencyStats
			_ = rows.Scan(&s.Currency, &s.InvoicedMTD, &s.InvoicedYTD, &s.Overdue)
			statsMap[s.Currency] = &s
		}
		rows.Close()

		// Per-currency collected payments
		pRows, err := db.Query(ctx, `
			SELECT
				p.currency,
				COALESCE(SUM(CASE WHEN p.paid_at >= $1 THEN p.amount ELSE 0 END), 0) AS collected_mtd,
				COALESCE(SUM(CASE WHEN p.paid_at >= $2 THEN p.amount ELSE 0 END), 0) AS collected_ytd
			FROM payment p
			GROUP BY p.currency
		`, monthStart, yearStart)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var currency string
				var mtd, ytd int64
				_ = pRows.Scan(&currency, &mtd, &ytd)
				if s, ok := statsMap[currency]; ok {
					s.CollectedMTD = mtd
					s.CollectedYTD = ytd
				} else {
					statsMap[currency] = &currencyStats{
						Currency:     currency,
						CollectedMTD: mtd,
						CollectedYTD: ytd,
					}
				}
			}
		}

		// Build result slice
		stats := make([]currencyStats, 0, len(statsMap))
		for _, s := range statsMap {
			stats = append(stats, *s)
		}
		if len(stats) == 0 {
			// Return zero stats for default currencies
			stats = []currencyStats{
				{Currency: "XOF"},
				{Currency: "USD"},
			}
		}

		// Invoice counts by status
		type statusCount struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		var counts []statusCount
		cRows, _ := db.Query(ctx, `
			SELECT status, COUNT(*) FROM invoice GROUP BY status ORDER BY status
		`)
		if cRows != nil {
			defer cRows.Close()
			for cRows.Next() {
				var sc statusCount
				_ = cRows.Scan(&sc.Status, &sc.Count)
				counts = append(counts, sc)
			}
		}
		if counts == nil {
			counts = []statusCount{}
		}

		// Monthly revenue trend (last 12 months, per currency)
		type monthRevenue struct {
			Month    string `json:"month"`
			Currency string `json:"currency"`
			Invoiced int64  `json:"invoiced"`
			Collected int64 `json:"collected"`
		}
		var trend []monthRevenue
		tRows, _ := db.Query(ctx, `
			SELECT
				TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS month,
				currency,
				COALESCE(SUM(CASE WHEN status != 'cancelled' THEN total ELSE 0 END), 0) AS invoiced
			FROM invoice
			WHERE created_at >= NOW() - INTERVAL '12 months'
			GROUP BY DATE_TRUNC('month', created_at), currency
			ORDER BY month DESC, currency
		`)
		if tRows != nil {
			defer tRows.Close()
			for tRows.Next() {
				var mr monthRevenue
				_ = tRows.Scan(&mr.Month, &mr.Currency, &mr.Invoiced)
				trend = append(trend, mr)
			}
		}
		if trend == nil {
			trend = []monthRevenue{}
		}

		// Top partners by revenue (YTD)
		type partnerRevenue struct {
			PartnerID   string `json:"partner_id"`
			PartnerName string `json:"partner_name"`
			Currency    string `json:"currency"`
			TotalYTD    int64  `json:"total_ytd"`
		}
		var topPartners []partnerRevenue
		prRows, _ := db.Query(ctx, `
			SELECT i.partner_id, p.name, i.currency,
				COALESCE(SUM(i.total), 0) AS total_ytd
			FROM invoice i
			JOIN partner p ON p.id = i.partner_id
			WHERE i.created_at >= $1 AND i.status != 'cancelled'
			GROUP BY i.partner_id, p.name, i.currency
			ORDER BY total_ytd DESC
			LIMIT 10
		`, yearStart)
		if prRows != nil {
			defer prRows.Close()
			for prRows.Next() {
				var pr partnerRevenue
				_ = prRows.Scan(&pr.PartnerID, &pr.PartnerName, &pr.Currency, &pr.TotalYTD)
				topPartners = append(topPartners, pr)
			}
		}
		if topPartners == nil {
			topPartners = []partnerRevenue{}
		}

		return c.JSON(fiber.Map{
			"by_currency":  stats,
			"status_counts": counts,
			"monthly_trend": trend,
			"top_partners": topPartners,
		})
	}
}

// ─── Invoice Types ───────────────────────────────────────────────────────────

type invoiceItem struct {
	ID          string  `json:"id,omitempty"`
	Section     *string `json:"section,omitempty"`
	Position    int     `json:"position"`
	Description string  `json:"description"`
	Quantity    int64   `json:"quantity"`
	UnitPrice   int64   `json:"unit_price"`
	Amount      int64   `json:"amount"`
}

type invoiceRow struct {
	ID            string        `json:"id"`
	InvoiceNumber string        `json:"invoice_number"`
	PartnerID     string        `json:"partner_id"`
	PartnerName   string        `json:"partner_name"`
	CreatedByID   string        `json:"created_by_id"`
	CreatedByName string        `json:"created_by_name"`
	Status        string        `json:"status"`
	Currency      string        `json:"currency"`
	Subtotal      int64         `json:"subtotal"`
	TaxRate       float64       `json:"tax_rate"`
	TaxAmount     int64         `json:"tax_amount"`
	Total         int64         `json:"total"`
	USDRate       *float64      `json:"usd_rate,omitempty"`
	USDTotal      *int64        `json:"usd_total,omitempty"`
	DueDate       *string       `json:"due_date,omitempty"`
	SentAt        *time.Time    `json:"sent_at,omitempty"`
	PaidAt        *time.Time    `json:"paid_at,omitempty"`
	Notes         *string       `json:"notes,omitempty"`
	PaymentTerms  *string       `json:"payment_terms,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Items         []invoiceItem `json:"items,omitempty"`
}

// ─── List Invoices ───────────────────────────────────────────────────────────

// ListInvoices GET /api/admin/billing/invoices
func ListInvoices(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := max(c.QueryInt("page", 1), 1)
		limit := min(c.QueryInt("limit", 20), 100)
		offset := (page - 1) * limit

		conditions := []string{"1=1"}
		args := []any{}
		idx := 1

		if s := c.Query("status"); s != "" {
			conditions = append(conditions, fmt.Sprintf("i.status = $%d", idx))
			args = append(args, s); idx++
		}
		if cur := c.Query("currency"); cur != "" {
			conditions = append(conditions, fmt.Sprintf("i.currency = $%d", idx))
			args = append(args, cur); idx++
		}
		if pid := c.Query("partner_id"); pid != "" {
			conditions = append(conditions, fmt.Sprintf("i.partner_id = $%d", idx))
			args = append(args, pid); idx++
		}
		if q := strings.TrimSpace(c.Query("q")); q != "" {
			conditions = append(conditions, fmt.Sprintf(
				"(i.invoice_number ILIKE $%d OR p.name ILIKE $%d)", idx, idx,
			))
			args = append(args, "%"+q+"%"); idx++
		}

		where := strings.Join(conditions, " AND ")

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		query := fmt.Sprintf(`
			SELECT
				i.id, i.invoice_number, i.partner_id, p.name,
				i.created_by, au.name,
				i.status, i.currency, i.subtotal, i.tax_rate, i.tax_amount, i.total,
				i.usd_rate, i.usd_total,
				TO_CHAR(i.due_date, 'YYYY-MM-DD'), i.sent_at, i.paid_at,
				i.notes, i.payment_terms, i.created_at, i.updated_at
			FROM invoice i
			JOIN partner p ON p.id = i.partner_id
			JOIN admin_user au ON au.id = i.created_by
			WHERE %s
			ORDER BY i.created_at DESC
			LIMIT $%d OFFSET $%d
		`, where, idx, idx+1)
		args = append(args, limit, offset)

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		invoices := []invoiceRow{}
		for rows.Next() {
			var inv invoiceRow
			_ = rows.Scan(
				&inv.ID, &inv.InvoiceNumber, &inv.PartnerID, &inv.PartnerName,
				&inv.CreatedByID, &inv.CreatedByName,
				&inv.Status, &inv.Currency, &inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total,
				&inv.USDRate, &inv.USDTotal,
				&inv.DueDate, &inv.SentAt, &inv.PaidAt,
				&inv.Notes, &inv.PaymentTerms, &inv.CreatedAt, &inv.UpdatedAt,
			)
			invoices = append(invoices, inv)
		}

		var total int
		countArgs := args[:len(args)-2]
		_ = db.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM invoice i JOIN partner p ON p.id = i.partner_id JOIN admin_user au ON au.id = i.created_by WHERE %s`, where),
			countArgs...,
		).Scan(&total)

		return c.JSON(fiber.Map{
			"data":  invoices,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// ─── Get Invoice ─────────────────────────────────────────────────────────────

// GetInvoice GET /api/admin/billing/invoices/:id
func GetInvoice(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var inv invoiceRow
		err := db.QueryRow(ctx, `
			SELECT
				i.id, i.invoice_number, i.partner_id, p.name,
				i.created_by, au.name,
				i.status, i.currency, i.subtotal, i.tax_rate, i.tax_amount, i.total,
				i.usd_rate, i.usd_total,
				TO_CHAR(i.due_date, 'YYYY-MM-DD'), i.sent_at, i.paid_at,
				i.notes, i.payment_terms, i.created_at, i.updated_at
			FROM invoice i
			JOIN partner p ON p.id = i.partner_id
			JOIN admin_user au ON au.id = i.created_by
			WHERE i.id = $1
		`, id).Scan(
			&inv.ID, &inv.InvoiceNumber, &inv.PartnerID, &inv.PartnerName,
			&inv.CreatedByID, &inv.CreatedByName,
			&inv.Status, &inv.Currency, &inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total,
			&inv.USDRate, &inv.USDTotal,
			&inv.DueDate, &inv.SentAt, &inv.PaidAt,
			&inv.Notes, &inv.PaymentTerms, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "invoice_not_found"})
		}

		// Load line items
		iRows, _ := db.Query(ctx, `
			SELECT id, section, position, description, quantity, unit_price, amount
			FROM invoice_item WHERE invoice_id = $1 ORDER BY position
		`, id)
		if iRows != nil {
			defer iRows.Close()
			for iRows.Next() {
				var item invoiceItem
				_ = iRows.Scan(&item.ID, &item.Section, &item.Position, &item.Description,
					&item.Quantity, &item.UnitPrice, &item.Amount)
				inv.Items = append(inv.Items, item)
			}
		}
		if inv.Items == nil {
			inv.Items = []invoiceItem{}
		}

		// Load payments
		type paymentSummary struct {
			ID        string    `json:"id"`
			Amount    int64     `json:"amount"`
			Currency  string    `json:"currency"`
			Method    string    `json:"method"`
			Reference *string   `json:"reference,omitempty"`
			PaidAt    time.Time `json:"paid_at"`
		}
		var payments []paymentSummary
		pmRows, _ := db.Query(ctx, `
			SELECT id, amount, currency, method, reference, paid_at
			FROM payment WHERE invoice_id = $1 ORDER BY paid_at DESC
		`, id)
		if pmRows != nil {
			defer pmRows.Close()
			for pmRows.Next() {
				var pm paymentSummary
				_ = pmRows.Scan(&pm.ID, &pm.Amount, &pm.Currency, &pm.Method, &pm.Reference, &pm.PaidAt)
				payments = append(payments, pm)
			}
		}
		if payments == nil {
			payments = []paymentSummary{}
		}

		return c.JSON(fiber.Map{
			"invoice":  inv,
			"payments": payments,
		})
	}
}

// ─── Create Invoice ──────────────────────────────────────────────────────────

type createInvoiceRequest struct {
	PartnerID    string        `json:"partner_id"`
	Currency     string        `json:"currency"`
	TaxRate      float64       `json:"tax_rate"`
	DueDate      *string       `json:"due_date"`
	Notes        *string       `json:"notes"`
	PaymentTerms *string       `json:"payment_terms"`
	USDRate      *float64      `json:"usd_rate"`
	Items        []invoiceItem `json:"items"`
}

// CreateInvoice POST /api/admin/billing/invoices
func CreateInvoice(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req createInvoiceRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.PartnerID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "partner_id required"})
		}
		if len(req.Items) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "at least one item required"})
		}
		if req.Currency == "" {
			req.Currency = "XOF"
		}

		// Compute totals
		var subtotal int64
		for _, it := range req.Items {
			subtotal += int64(it.Quantity) * it.UnitPrice
		}
		taxAmount := int64(float64(subtotal) * req.TaxRate / 100)
		total := subtotal + taxAmount

		// USD equivalent
		var usdTotal *int64
		if req.USDRate != nil && *req.USDRate > 0 && req.Currency == "XOF" {
			v := int64(float64(total) / *req.USDRate * 100) // in USD cents
			usdTotal = &v
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		invoiceNum, err := nextInvoiceNumber(ctx, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate invoice number"})
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer tx.Rollback(ctx)

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO invoice
				(invoice_number, partner_id, created_by, status, currency,
				 subtotal, tax_rate, tax_amount, total, usd_rate, usd_total,
				 due_date, notes, payment_terms)
			VALUES ($1,$2,$3,'draft',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING id
		`, invoiceNum, req.PartnerID, actor.AdminID, req.Currency,
			subtotal, req.TaxRate, taxAmount, total,
			req.USDRate, usdTotal, req.DueDate, req.Notes, req.PaymentTerms,
		).Scan(&id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		for pos, it := range req.Items {
			amt := int64(it.Quantity) * it.UnitPrice
			_, err = tx.Exec(ctx, `
				INSERT INTO invoice_item (invoice_id, position, description, quantity, unit_price, amount)
				VALUES ($1,$2,$3,$4,$5,$6)
			`, id, pos+1, it.Description, it.Quantity, it.UnitPrice, amt)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
		}

		if err = tx.Commit(ctx); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ActorUA:      c.Get("User-Agent"),
			ResourceType: "invoice",
			ResourceID:   id,
			Action:       audit.ActionCreate,
			RequestID:    reqID,
			AfterState: audit.Sanitize(map[string]any{
				"invoice_number": invoiceNum,
				"partner_id":     req.PartnerID,
				"currency":       req.Currency,
				"total":          total,
			}),
		})

		return c.Status(201).JSON(fiber.Map{
			"id":             id,
			"invoice_number": invoiceNum,
			"total":          total,
		})
	}
}

// ─── Update Invoice ──────────────────────────────────────────────────────────

type updateInvoiceRequest struct {
	Notes        *string  `json:"notes"`
	PaymentTerms *string  `json:"payment_terms"`
	DueDate      *string  `json:"due_date"`
	USDRate      *float64 `json:"usd_rate"`
}

// UpdateInvoice PATCH /api/admin/billing/invoices/:id
func UpdateInvoice(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req updateInvoiceRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Only draft invoices can be edited
		var status string
		_ = db.QueryRow(ctx, `SELECT status FROM invoice WHERE id = $1`, id).Scan(&status)
		if status == "" {
			return c.Status(404).JSON(fiber.Map{"error": "invoice_not_found"})
		}
		if status != "draft" {
			return c.Status(409).JSON(fiber.Map{"error": "only_draft_invoices_can_be_edited"})
		}

		sets := []string{}
		args := []any{}
		idx := 1

		if req.Notes != nil {
			sets = append(sets, fmt.Sprintf("notes = $%d", idx)); args = append(args, *req.Notes); idx++
		}
		if req.PaymentTerms != nil {
			sets = append(sets, fmt.Sprintf("payment_terms = $%d", idx)); args = append(args, *req.PaymentTerms); idx++
		}
		if req.DueDate != nil {
			sets = append(sets, fmt.Sprintf("due_date = $%d", idx)); args = append(args, *req.DueDate); idx++
		}
		if req.USDRate != nil {
			sets = append(sets, fmt.Sprintf("usd_rate = $%d", idx)); args = append(args, *req.USDRate); idx++
		}
		if len(sets) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "no fields to update"})
		}

		args = append(args, id)
		_, err := db.Exec(ctx,
			fmt.Sprintf("UPDATE invoice SET %s WHERE id = $%d", strings.Join(sets, ", "), idx),
			args...,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "invoice",
			ResourceID:   id,
			Action:       audit.ActionUpdate,
			RequestID:    reqID,
			AfterState:   req,
		})

		return c.SendStatus(204)
	}
}

// ─── Invoice Actions ─────────────────────────────────────────────────────────

// SendInvoice POST /api/admin/billing/invoices/:id/send
func SendInvoice(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := db.Exec(ctx, `
			UPDATE invoice SET status = 'sent', sent_at = NOW()
			WHERE id = $1 AND status = 'draft'
		`, id)
		if err != nil || res.RowsAffected() == 0 {
			return c.Status(409).JSON(fiber.Map{"error": "invoice_not_in_draft_state"})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "invoice",
			ResourceID:   id,
			Action:       "SEND",
			RequestID:    reqID,
		})

		return c.JSON(fiber.Map{"status": "sent"})
	}
}

// CancelInvoice POST /api/admin/billing/invoices/:id/cancel
func CancelInvoice(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := db.Exec(ctx, `
			UPDATE invoice SET status = 'cancelled'
			WHERE id = $1 AND status IN ('draft','sent','overdue')
		`, id)
		if err != nil || res.RowsAffected() == 0 {
			return c.Status(409).JSON(fiber.Map{"error": "invoice_cannot_be_cancelled"})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "invoice",
			ResourceID:   id,
			Action:       "CANCEL",
			RequestID:    reqID,
		})

		return c.JSON(fiber.Map{"status": "cancelled"})
	}
}

// ─── Payment ─────────────────────────────────────────────────────────────────

type recordPaymentRequest struct {
	Amount    int64   `json:"amount"`
	Currency  string  `json:"currency"`
	Method    string  `json:"method"`
	Reference *string `json:"reference"`
	Notes     *string `json:"notes"`
	PaidAt    *string `json:"paid_at"` // RFC3339 or YYYY-MM-DD
}

// RecordPayment POST /api/admin/billing/invoices/:id/payments
func RecordPayment(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		invoiceID := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req recordPaymentRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Amount <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "amount must be positive"})
		}
		if req.Currency == "" {
			req.Currency = "XOF"
		}
		if req.Method == "" {
			req.Method = "bank_transfer"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var invStatus string
		var invTotal int64
		_ = db.QueryRow(ctx, `SELECT status, total FROM invoice WHERE id = $1`, invoiceID).
			Scan(&invStatus, &invTotal)
		if invStatus == "" {
			return c.Status(404).JSON(fiber.Map{"error": "invoice_not_found"})
		}
		if invStatus == "cancelled" || invStatus == "refunded" {
			return c.Status(409).JSON(fiber.Map{"error": "cannot_pay_cancelled_invoice"})
		}

		paidAt := time.Now()
		if req.PaidAt != nil {
			if t, err := time.Parse("2006-01-02", *req.PaidAt); err == nil {
				paidAt = t
			}
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer tx.Rollback(ctx)

		var payID string
		err = tx.QueryRow(ctx, `
			INSERT INTO payment (invoice_id, created_by, amount, currency, method, reference, notes, paid_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING id
		`, invoiceID, actor.AdminID, req.Amount, req.Currency, req.Method,
			req.Reference, req.Notes, paidAt,
		).Scan(&payID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Check if invoice is fully paid
		var totalPaid int64
		_ = tx.QueryRow(ctx,
			`SELECT COALESCE(SUM(amount), 0) FROM payment WHERE invoice_id = $1`, invoiceID,
		).Scan(&totalPaid)

		if totalPaid >= invTotal {
			_, _ = tx.Exec(ctx,
				`UPDATE invoice SET status = 'paid', paid_at = $1 WHERE id = $2`,
				paidAt, invoiceID,
			)
		}

		if err = tx.Commit(ctx); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "payment",
			ResourceID:   payID,
			Action:       audit.ActionCreate,
			RequestID:    reqID,
			AfterState: audit.Sanitize(map[string]any{
				"invoice_id": invoiceID,
				"amount":     req.Amount,
				"currency":   req.Currency,
				"method":     req.Method,
			}),
		})

		return c.Status(201).JSON(fiber.Map{
			"id":          payID,
			"total_paid":  totalPaid,
			"fully_paid":  totalPaid >= invTotal,
		})
	}
}

// ListPayments GET /api/admin/billing/payments
func ListPayments(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := max(c.QueryInt("page", 1), 1)
		limit := min(c.QueryInt("limit", 20), 100)
		offset := (page - 1) * limit

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		conditions := []string{"1=1"}
		args := []any{}
		idx := 1

		if cur := c.Query("currency"); cur != "" {
			conditions = append(conditions, fmt.Sprintf("p.currency = $%d", idx))
			args = append(args, cur); idx++
		}
		if method := c.Query("method"); method != "" {
			conditions = append(conditions, fmt.Sprintf("p.method = $%d", idx))
			args = append(args, method); idx++
		}

		where := strings.Join(conditions, " AND ")

		query := fmt.Sprintf(`
			SELECT
				p.id, p.invoice_id, i.invoice_number, i.partner_id, pt.name AS partner_name,
				p.amount, p.currency, p.method, p.reference, p.notes,
				p.paid_at, p.created_at,
				au.name AS recorded_by
			FROM payment p
			JOIN invoice i ON i.id = p.invoice_id
			JOIN partner pt ON pt.id = i.partner_id
			JOIN admin_user au ON au.id = p.created_by
			WHERE %s
			ORDER BY p.paid_at DESC
			LIMIT $%d OFFSET $%d
		`, where, idx, idx+1)
		args = append(args, limit, offset)

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		type payRow struct {
			ID            string    `json:"id"`
			InvoiceID     string    `json:"invoice_id"`
			InvoiceNumber string    `json:"invoice_number"`
			PartnerID     string    `json:"partner_id"`
			PartnerName   string    `json:"partner_name"`
			Amount        int64     `json:"amount"`
			Currency      string    `json:"currency"`
			Method        string    `json:"method"`
			Reference     *string   `json:"reference,omitempty"`
			Notes         *string   `json:"notes,omitempty"`
			PaidAt        time.Time `json:"paid_at"`
			CreatedAt     time.Time `json:"created_at"`
			RecordedBy    string    `json:"recorded_by"`
		}

		payments := []payRow{}
		for rows.Next() {
			var p payRow
			_ = rows.Scan(
				&p.ID, &p.InvoiceID, &p.InvoiceNumber, &p.PartnerID, &p.PartnerName,
				&p.Amount, &p.Currency, &p.Method, &p.Reference, &p.Notes,
				&p.PaidAt, &p.CreatedAt, &p.RecordedBy,
			)
			payments = append(payments, p)
		}

		var total int
		_ = db.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM payment p JOIN invoice i ON i.id = p.invoice_id JOIN partner pt ON pt.id = i.partner_id JOIN admin_user au ON au.id = p.created_by WHERE %s`, where),
			args[:len(args)-2]...,
		).Scan(&total)

		return c.JSON(fiber.Map{
			"data":  payments,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// ─── Devis (Quotes) Types ─────────────────────────────────────────────────────

type devisRow struct {
	ID                  string        `json:"id"`
	DevisNumber         string        `json:"devis_number"`
	PartnerID           string        `json:"partner_id"`
	PartnerName         string        `json:"partner_name"`
	CreatedByID         string        `json:"created_by_id"`
	CreatedByName       string        `json:"created_by_name"`
	Status              string        `json:"status"`
	Currency            string        `json:"currency"`
	Subtotal            int64         `json:"subtotal"`
	TaxRate             float64       `json:"tax_rate"`
	TaxAmount           int64         `json:"tax_amount"`
	Total               int64         `json:"total"`
	USDRate             *float64      `json:"usd_rate,omitempty"`
	USDTotal            *int64        `json:"usd_total,omitempty"`
	ValidUntil          *string       `json:"valid_until,omitempty"`
	SentAt              *time.Time    `json:"sent_at,omitempty"`
	RespondedAt         *time.Time    `json:"responded_at,omitempty"`
	ConvertedInvoiceID  *string       `json:"converted_invoice_id,omitempty"`
	Notes               *string       `json:"notes,omitempty"`
	Terms               *string       `json:"terms,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	Items               []invoiceItem `json:"items,omitempty"`
}

// ─── List Devis ───────────────────────────────────────────────────────────────

// ListDevis GET /api/admin/billing/devis
func ListDevis(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := max(c.QueryInt("page", 1), 1)
		limit := min(c.QueryInt("limit", 20), 100)
		offset := (page - 1) * limit

		conditions := []string{"1=1"}
		args := []any{}
		idx := 1

		if s := c.Query("status"); s != "" {
			conditions = append(conditions, fmt.Sprintf("d.status = $%d", idx))
			args = append(args, s); idx++
		}
		if cur := c.Query("currency"); cur != "" {
			conditions = append(conditions, fmt.Sprintf("d.currency = $%d", idx))
			args = append(args, cur); idx++
		}
		if pid := c.Query("partner_id"); pid != "" {
			conditions = append(conditions, fmt.Sprintf("d.partner_id = $%d", idx))
			args = append(args, pid); idx++
		}

		where := strings.Join(conditions, " AND ")

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		query := fmt.Sprintf(`
			SELECT
				d.id, d.devis_number, d.partner_id, p.name,
				d.created_by, au.name,
				d.status, d.currency, d.subtotal, d.tax_rate, d.tax_amount, d.total,
				d.usd_rate, d.usd_total,
				TO_CHAR(d.valid_until, 'YYYY-MM-DD'), d.sent_at, d.responded_at,
				d.converted_invoice_id, d.notes, d.terms, d.created_at, d.updated_at
			FROM devis d
			JOIN partner p ON p.id = d.partner_id
			JOIN admin_user au ON au.id = d.created_by
			WHERE %s
			ORDER BY d.created_at DESC
			LIMIT $%d OFFSET $%d
		`, where, idx, idx+1)
		args = append(args, limit, offset)

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		devisList := []devisRow{}
		for rows.Next() {
			var d devisRow
			_ = rows.Scan(
				&d.ID, &d.DevisNumber, &d.PartnerID, &d.PartnerName,
				&d.CreatedByID, &d.CreatedByName,
				&d.Status, &d.Currency, &d.Subtotal, &d.TaxRate, &d.TaxAmount, &d.Total,
				&d.USDRate, &d.USDTotal,
				&d.ValidUntil, &d.SentAt, &d.RespondedAt,
				&d.ConvertedInvoiceID, &d.Notes, &d.Terms, &d.CreatedAt, &d.UpdatedAt,
			)
			devisList = append(devisList, d)
		}

		var total int
		_ = db.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM devis d JOIN partner p ON p.id = d.partner_id JOIN admin_user au ON au.id = d.created_by WHERE %s`, where),
			args[:len(args)-2]...,
		).Scan(&total)

		return c.JSON(fiber.Map{
			"data":  devisList,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// ─── Get Devis ────────────────────────────────────────────────────────────────

// GetDevis GET /api/admin/billing/devis/:id
func GetDevis(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var d devisRow
		err := db.QueryRow(ctx, `
			SELECT
				d.id, d.devis_number, d.partner_id, p.name,
				d.created_by, au.name,
				d.status, d.currency, d.subtotal, d.tax_rate, d.tax_amount, d.total,
				d.usd_rate, d.usd_total,
				TO_CHAR(d.valid_until, 'YYYY-MM-DD'), d.sent_at, d.responded_at,
				d.converted_invoice_id, d.notes, d.terms, d.created_at, d.updated_at
			FROM devis d
			JOIN partner p ON p.id = d.partner_id
			JOIN admin_user au ON au.id = d.created_by
			WHERE d.id = $1
		`, id).Scan(
			&d.ID, &d.DevisNumber, &d.PartnerID, &d.PartnerName,
			&d.CreatedByID, &d.CreatedByName,
			&d.Status, &d.Currency, &d.Subtotal, &d.TaxRate, &d.TaxAmount, &d.Total,
			&d.USDRate, &d.USDTotal,
			&d.ValidUntil, &d.SentAt, &d.RespondedAt,
			&d.ConvertedInvoiceID, &d.Notes, &d.Terms, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "devis_not_found"})
		}

		iRows, _ := db.Query(ctx, `
			SELECT id, section, position, description, quantity, unit_price, amount
			FROM devis_item WHERE devis_id = $1 ORDER BY position
		`, id)
		if iRows != nil {
			defer iRows.Close()
			for iRows.Next() {
				var item invoiceItem
				_ = iRows.Scan(&item.ID, &item.Section, &item.Position, &item.Description,
					&item.Quantity, &item.UnitPrice, &item.Amount)
				d.Items = append(d.Items, item)
			}
		}
		if d.Items == nil {
			d.Items = []invoiceItem{}
		}

		return c.JSON(d)
	}
}

// ─── Create Devis ─────────────────────────────────────────────────────────────

type createDevisRequest struct {
	PartnerID  string        `json:"partner_id"`
	Currency   string        `json:"currency"`
	TaxRate    float64       `json:"tax_rate"`
	ValidUntil *string       `json:"valid_until"`
	Notes      *string       `json:"notes"`
	Terms      *string       `json:"terms"`
	USDRate    *float64      `json:"usd_rate"`
	Items      []invoiceItem `json:"items"`
}

// CreateDevis POST /api/admin/billing/devis
func CreateDevis(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req createDevisRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.PartnerID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "partner_id required"})
		}
		if len(req.Items) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "at least one item required"})
		}
		if req.Currency == "" {
			req.Currency = "XOF"
		}

		var subtotal int64
		for _, it := range req.Items {
			subtotal += int64(it.Quantity) * it.UnitPrice
		}
		taxAmount := int64(float64(subtotal) * req.TaxRate / 100)
		total := subtotal + taxAmount

		var usdTotal *int64
		if req.USDRate != nil && *req.USDRate > 0 && req.Currency == "XOF" {
			v := int64(float64(total) / *req.USDRate * 100)
			usdTotal = &v
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		devisNum, err := nextDevisNumber(ctx, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate devis number"})
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer tx.Rollback(ctx)

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO devis
				(devis_number, partner_id, created_by, status, currency,
				 subtotal, tax_rate, tax_amount, total, usd_rate, usd_total,
				 valid_until, notes, terms)
			VALUES ($1,$2,$3,'draft',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING id
		`, devisNum, req.PartnerID, actor.AdminID, req.Currency,
			subtotal, req.TaxRate, taxAmount, total,
			req.USDRate, usdTotal, req.ValidUntil, req.Notes, req.Terms,
		).Scan(&id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		for pos, it := range req.Items {
			amt := int64(it.Quantity) * it.UnitPrice
			_, err = tx.Exec(ctx, `
				INSERT INTO devis_item (devis_id, position, description, quantity, unit_price, amount)
				VALUES ($1,$2,$3,$4,$5,$6)
			`, id, pos+1, it.Description, it.Quantity, it.UnitPrice, amt)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
		}

		if err = tx.Commit(ctx); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ActorUA:      c.Get("User-Agent"),
			ResourceType: "devis",
			ResourceID:   id,
			Action:       audit.ActionCreate,
			RequestID:    reqID,
			AfterState: audit.Sanitize(map[string]any{
				"devis_number": devisNum,
				"partner_id":   req.PartnerID,
				"currency":     req.Currency,
				"total":        total,
			}),
		})

		return c.Status(201).JSON(fiber.Map{
			"id":           id,
			"devis_number": devisNum,
			"total":        total,
		})
	}
}

// ─── Devis Actions ────────────────────────────────────────────────────────────

// SendDevis POST /api/admin/billing/devis/:id/send
func SendDevis(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := db.Exec(ctx, `
			UPDATE devis SET status = 'sent', sent_at = NOW()
			WHERE id = $1 AND status = 'draft'
		`, id)
		if err != nil || res.RowsAffected() == 0 {
			return c.Status(409).JSON(fiber.Map{"error": "devis_not_in_draft_state"})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType: audit.EventAdminAction, ActorType: audit.ActorAdmin,
			ActorID: actor.AdminID, ActorIP: c.IP(),
			ResourceType: "devis", ResourceID: id, Action: "SEND", RequestID: reqID,
		})
		return c.JSON(fiber.Map{"status": "sent"})
	}
}

// RespondDevis POST /api/admin/billing/devis/:id/respond
// Body: {"accepted": true}
func RespondDevis(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var body struct {
			Accepted bool `json:"accepted"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}

		newStatus := "declined"
		if body.Accepted {
			newStatus = "accepted"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := db.Exec(ctx, `
			UPDATE devis SET status = $1, responded_at = NOW()
			WHERE id = $2 AND status = 'sent'
		`, newStatus, id)
		if err != nil || res.RowsAffected() == 0 {
			return c.Status(409).JSON(fiber.Map{"error": "devis_not_in_sent_state"})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType: audit.EventAdminAction, ActorType: audit.ActorAdmin,
			ActorID: actor.AdminID, ActorIP: c.IP(),
			ResourceType: "devis", ResourceID: id, Action: "RESPOND", RequestID: reqID,
			AfterState: map[string]any{"accepted": body.Accepted},
		})
		return c.JSON(fiber.Map{"status": newStatus})
	}
}

// ConvertDevisToInvoice POST /api/admin/billing/devis/:id/convert
// Creates a new invoice from an accepted devis.
func ConvertDevisToInvoice(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Load devis
		var d devisRow
		err := db.QueryRow(ctx, `
			SELECT id, partner_id, status, currency, subtotal, tax_rate, tax_amount, total,
				   usd_rate, usd_total, notes
			FROM devis WHERE id = $1
		`, id).Scan(
			&d.ID, &d.PartnerID, &d.Status, &d.Currency,
			&d.Subtotal, &d.TaxRate, &d.TaxAmount, &d.Total,
			&d.USDRate, &d.USDTotal, &d.Notes,
		)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "devis_not_found"})
		}
		if d.Status != "accepted" {
			return c.Status(409).JSON(fiber.Map{"error": "only_accepted_devis_can_be_converted"})
		}

		invoiceNum, err := nextInvoiceNumber(ctx, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate invoice number"})
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer tx.Rollback(ctx)

		var invoiceID string
		err = tx.QueryRow(ctx, `
			INSERT INTO invoice
				(invoice_number, partner_id, created_by, status, currency,
				 subtotal, tax_rate, tax_amount, total, usd_rate, usd_total, notes)
			VALUES ($1,$2,$3,'draft',$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id
		`, invoiceNum, d.PartnerID, actor.AdminID, d.Currency,
			d.Subtotal, d.TaxRate, d.TaxAmount, d.Total,
			d.USDRate, d.USDTotal, d.Notes,
		).Scan(&invoiceID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Copy items
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_item (invoice_id, position, description, quantity, unit_price, amount)
			SELECT $1, position, description, quantity, unit_price, amount
			FROM devis_item WHERE devis_id = $2
		`, invoiceID, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Mark devis as converted
		_, err = tx.Exec(ctx, `
			UPDATE devis SET status = 'converted', converted_invoice_id = $1 WHERE id = $2
		`, invoiceID, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		if err = tx.Commit(ctx); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAdminAction,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "devis",
			ResourceID:   id,
			Action:       "CONVERT",
			RequestID:    reqID,
			AfterState: map[string]any{
				"invoice_id":     invoiceID,
				"invoice_number": invoiceNum,
			},
		})

		return c.Status(201).JSON(fiber.Map{
			"invoice_id":     invoiceID,
			"invoice_number": invoiceNum,
		})
	}
}

// ─── Mark Overdue (background / manual trigger) ───────────────────────────────

// MarkOverdueInvoices POST /api/admin/billing/invoices/mark-overdue
// Superadmin only — transitions sent invoices past due_date to overdue.
func MarkOverdueInvoices(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		res, err := db.Exec(ctx, `
			UPDATE invoice
			SET status = 'overdue'
			WHERE status = 'sent'
			  AND due_date IS NOT NULL
			  AND due_date < CURRENT_DATE
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"updated": res.RowsAffected()})
	}
}
