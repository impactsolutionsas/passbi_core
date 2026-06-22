package admin

import (
	"context"
	"encoding/csv"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// avgDistanceKm: estimated average transit journey distance in Dakar
// avgTaxiGperKm: 180 g CO2/km for taxi
// avgTransitGperKm: 35 g CO2/km (weighted average across BRT/TER/DDD/AFTU)
const (
	avgDistanceKm      = 7.0
	avgTaxiGperKm      = 180.0
	avgTransitGperKm   = 35.0
	avgSavedGperJourney = (avgTaxiGperKm - avgTransitGperKm) * avgDistanceKm // 1015g
)

type CO2Stats struct {
	TotalJourneys    int64   `json:"total_journeys"`
	JourneysToday    int64   `json:"journeys_today"`
	JourneysMonth    int64   `json:"journeys_month"`
	CO2SavedKgTotal  float64 `json:"co2_saved_kg_total"`
	CO2SavedKgToday  float64 `json:"co2_saved_kg_today"`
	CO2SavedKgMonth  float64 `json:"co2_saved_kg_month"`
	TreesEquivalent  float64 `json:"trees_equivalent"`
	AvgPerPartner    float64 `json:"avg_per_partner"`
	TopPartners      []CO2Partner  `json:"top_partners"`
	DailyTrend       []CO2Day      `json:"daily_trend"`
}

type CO2Partner struct {
	Name     string  `json:"name"`
	Journeys int64   `json:"journeys"`
	CO2KgSaved float64 `json:"co2_kg_saved"`
}

type CO2Day struct {
	Date     string  `json:"date"`
	Journeys int64   `json:"journeys"`
	CO2KgSaved float64 `json:"co2_kg_saved"`
}

func CO2Overview(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := context.Background()

		var stats CO2Stats

		// Total / today / month journey counts (route-search calls)
		err := db.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%') AS total,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%' AND timestamp >= CURRENT_DATE) AS today,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%' AND timestamp >= date_trunc('month', NOW())) AS month
			FROM usage_log
		`).Scan(&stats.TotalJourneys, &stats.JourneysToday, &stats.JourneysMonth)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		stats.CO2SavedKgTotal = float64(stats.TotalJourneys) * avgSavedGperJourney / 1000.0
		stats.CO2SavedKgToday = float64(stats.JourneysToday) * avgSavedGperJourney / 1000.0
		stats.CO2SavedKgMonth = float64(stats.JourneysMonth) * avgSavedGperJourney / 1000.0
		// 1 tree absorbs ~21kg CO2/year
		stats.TreesEquivalent = stats.CO2SavedKgTotal / 21.0

		// Count active partners to compute avg
		var partnerCount int64
		_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM partner WHERE status = 'active'`).Scan(&partnerCount)
		if partnerCount > 0 {
			stats.AvgPerPartner = stats.CO2SavedKgMonth / float64(partnerCount)
		}

		// Top 5 partners by journey count this month
		rows, err := db.Query(ctx, `
			SELECT p.name, COUNT(*) AS journeys
			FROM usage_log u
			JOIN partner p ON p.id = u.partner_id
			WHERE u.endpoint LIKE '%route-search%'
			  AND u.timestamp >= date_trunc('month', NOW())
			GROUP BY p.name
			ORDER BY journeys DESC
			LIMIT 5
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cp CO2Partner
				if err := rows.Scan(&cp.Name, &cp.Journeys); err == nil {
					cp.CO2KgSaved = float64(cp.Journeys) * avgSavedGperJourney / 1000.0
					stats.TopPartners = append(stats.TopPartners, cp)
				}
			}
		}

		// Daily trend last 30 days
		trendRows, err := db.Query(ctx, `
			SELECT
				DATE(timestamp) AS day,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%') AS journeys
			FROM usage_log
			WHERE timestamp >= NOW() - INTERVAL '30 days'
			GROUP BY day
			ORDER BY day
		`)
		if err == nil {
			defer trendRows.Close()
			for trendRows.Next() {
				var d CO2Day
				if err := trendRows.Scan(&d.Date, &d.Journeys); err == nil {
					d.CO2KgSaved = float64(d.Journeys) * avgSavedGperJourney / 1000.0
					stats.DailyTrend = append(stats.DailyTrend, d)
				}
			}
		}

		if stats.TopPartners == nil {
			stats.TopPartners = []CO2Partner{}
		}
		if stats.DailyTrend == nil {
			stats.DailyTrend = []CO2Day{}
		}

		return c.JSON(stats)
	}
}

// ── Report endpoint ───────────────────────────────────────────────────────────

type CO2MonthRow struct {
	Month      string  `json:"month"`
	Journeys   int64   `json:"journeys"`
	CO2KgSaved float64 `json:"co2_kg_saved"`
	TaxiKgEq   float64 `json:"taxi_kg_eq"`
}

type CO2PartnerRow struct {
	Name       string  `json:"name"`
	Email      string  `json:"email"`
	Tier       string  `json:"tier"`
	Journeys   int64   `json:"journeys"`
	CO2KgSaved float64 `json:"co2_kg_saved"`
	SharePct   float64 `json:"share_pct"`
}

type CO2ReportData struct {
	From        string          `json:"from"`
	To          string          `json:"to"`
	TotalJourneys  int64        `json:"total_journeys"`
	CO2KgSaved     float64      `json:"co2_kg_saved"`
	TaxiKgEquiv    float64      `json:"taxi_kg_equiv"`
	TreesEq        float64      `json:"trees_eq"`
	CarsOffRoad    float64      `json:"cars_off_road"` // 1 car ≈ 4600 kg CO2/year
	ByMonth     []CO2MonthRow   `json:"by_month"`
	ByPartner   []CO2PartnerRow `json:"by_partner"`
	DailyTrend  []CO2Day        `json:"daily_trend"`
}

// CO2Report GET /api/admin/co2/report?from=YYYY-MM-DD&to=YYYY-MM-DD
func CO2Report(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		fromStr := c.Query("from")
		toStr := c.Query("to")

		// Defaults: current year
		now := time.Now()
		from := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		to := now

		if fromStr != "" {
			if t, err := time.Parse("2006-01-02", fromStr); err == nil {
				from = t
			}
		}
		if toStr != "" {
			if t, err := time.Parse("2006-01-02", toStr); err == nil {
				to = t.Add(24*time.Hour - time.Second)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		report := CO2ReportData{
			From: from.Format("2006-01-02"),
			To:   to.Format("2006-01-02"),
		}

		// Total
		_ = db.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_log
			WHERE endpoint LIKE '%route-search%' AND timestamp BETWEEN $1 AND $2
		`, from, to).Scan(&report.TotalJourneys)

		report.CO2KgSaved = float64(report.TotalJourneys) * avgSavedGperJourney / 1000.0
		report.TaxiKgEquiv = float64(report.TotalJourneys) * avgTaxiGperKm * avgDistanceKm / 1000.0
		report.TreesEq = report.CO2KgSaved / 21.0
		report.CarsOffRoad = report.CO2KgSaved / 4600.0

		// By month
		monthRows, err := db.Query(ctx, `
			SELECT
				TO_CHAR(DATE_TRUNC('month', timestamp), 'YYYY-MM') AS month,
				COUNT(*) AS journeys
			FROM usage_log
			WHERE endpoint LIKE '%route-search%' AND timestamp BETWEEN $1 AND $2
			GROUP BY month ORDER BY month
		`, from, to)
		if err == nil {
			defer monthRows.Close()
			for monthRows.Next() {
				var r CO2MonthRow
				if monthRows.Scan(&r.Month, &r.Journeys) == nil {
					r.CO2KgSaved = float64(r.Journeys) * avgSavedGperJourney / 1000.0
					r.TaxiKgEq = float64(r.Journeys) * avgTaxiGperKm * avgDistanceKm / 1000.0
					report.ByMonth = append(report.ByMonth, r)
				}
			}
		}

		// By partner
		partnerRows, err := db.Query(ctx, `
			SELECT p.name, p.email, p.tier, COUNT(*) AS journeys
			FROM usage_log u
			JOIN partner p ON p.id = u.partner_id
			WHERE u.endpoint LIKE '%route-search%' AND u.timestamp BETWEEN $1 AND $2
			GROUP BY p.name, p.email, p.tier
			ORDER BY journeys DESC
		`, from, to)
		if err == nil {
			defer partnerRows.Close()
			for partnerRows.Next() {
				var r CO2PartnerRow
				if partnerRows.Scan(&r.Name, &r.Email, &r.Tier, &r.Journeys) == nil {
					r.CO2KgSaved = float64(r.Journeys) * avgSavedGperJourney / 1000.0
					if report.TotalJourneys > 0 {
						r.SharePct = float64(r.Journeys) / float64(report.TotalJourneys) * 100
					}
					report.ByPartner = append(report.ByPartner, r)
				}
			}
		}

		// Daily trend
		trendRows, err := db.Query(ctx, `
			SELECT DATE(timestamp) AS day, COUNT(*) AS journeys
			FROM usage_log
			WHERE endpoint LIKE '%route-search%' AND timestamp BETWEEN $1 AND $2
			GROUP BY day ORDER BY day
		`, from, to)
		if err == nil {
			defer trendRows.Close()
			for trendRows.Next() {
				var d CO2Day
				if trendRows.Scan(&d.Date, &d.Journeys) == nil {
					d.CO2KgSaved = float64(d.Journeys) * avgSavedGperJourney / 1000.0
					report.DailyTrend = append(report.DailyTrend, d)
				}
			}
		}

		if report.ByMonth == nil {
			report.ByMonth = []CO2MonthRow{}
		}
		if report.ByPartner == nil {
			report.ByPartner = []CO2PartnerRow{}
		}
		if report.DailyTrend == nil {
			report.DailyTrend = []CO2Day{}
		}

		return c.JSON(report)
	}
}

// CO2ExportCSV GET /api/admin/co2/export?from=&to= — CSV for Excel compatibility
func CO2ExportCSV(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Reuse report logic inline
		// call the DB directly
		fromStr := c.Query("from")
		toStr := c.Query("to")
		now := time.Now()
		from := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		to := now
		if fromStr != "" {
			if t, err := time.Parse("2006-01-02", fromStr); err == nil {
				from = t
			}
		}
		if toStr != "" {
			if t, err := time.Parse("2006-01-02", toStr); err == nil {
				to = t.Add(24*time.Hour - time.Second)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		filename := fmt.Sprintf("co2-report-%s-%s.csv", from.Format("20060102"), to.Format("20060102"))
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)

		w := csv.NewWriter(c.Response().BodyWriter())

		// Section 1: by partner
		_ = w.Write([]string{"Section", "Partenaire", "Email", "Tier", "Trajets", "CO2 évité (kg)", "CO2 taxi équiv. (kg)", "Part (%)"})

		var totalJourneys int64
		_ = db.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_log
			WHERE endpoint LIKE '%route-search%' AND timestamp BETWEEN $1 AND $2
		`, from, to).Scan(&totalJourneys)

		rows, err := db.Query(ctx, `
			SELECT p.name, p.email, p.tier, COUNT(*) AS journeys
			FROM usage_log u
			JOIN partner p ON p.id = u.partner_id
			WHERE u.endpoint LIKE '%route-search%' AND u.timestamp BETWEEN $1 AND $2
			GROUP BY p.name, p.email, p.tier ORDER BY journeys DESC
		`, from, to)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name, email, tier string
				var journeys int64
				if rows.Scan(&name, &email, &tier, &journeys) == nil {
					co2 := float64(journeys) * avgSavedGperJourney / 1000.0
					taxiEq := float64(journeys) * avgTaxiGperKm * avgDistanceKm / 1000.0
					share := 0.0
					if totalJourneys > 0 {
						share = float64(journeys) / float64(totalJourneys) * 100
					}
					_ = w.Write([]string{
						"Partenaire", name, email, tier,
						fmt.Sprintf("%d", journeys),
						fmt.Sprintf("%.2f", co2),
						fmt.Sprintf("%.2f", taxiEq),
						fmt.Sprintf("%.1f", share),
					})
				}
			}
		}

		// Section 2: by month
		_ = w.Write([]string{})
		_ = w.Write([]string{"Section", "Mois", "", "", "Trajets", "CO2 évité (kg)", "CO2 taxi équiv. (kg)", ""})

		mRows, err := db.Query(ctx, `
			SELECT TO_CHAR(DATE_TRUNC('month', timestamp), 'YYYY-MM'), COUNT(*)
			FROM usage_log
			WHERE endpoint LIKE '%route-search%' AND timestamp BETWEEN $1 AND $2
			GROUP BY 1 ORDER BY 1
		`, from, to)
		if err == nil {
			defer mRows.Close()
			for mRows.Next() {
				var month string
				var journeys int64
				if mRows.Scan(&month, &journeys) == nil {
					co2 := float64(journeys) * avgSavedGperJourney / 1000.0
					taxiEq := float64(journeys) * avgTaxiGperKm * avgDistanceKm / 1000.0
					_ = w.Write([]string{
						"Mensuel", month, "", "",
						fmt.Sprintf("%d", journeys),
						fmt.Sprintf("%.2f", co2),
						fmt.Sprintf("%.2f", taxiEq), "",
					})
				}
			}
		}

		w.Flush()
		return nil
	}
}
