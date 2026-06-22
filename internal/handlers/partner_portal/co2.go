package partner_portal

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	partnerAvgDistanceKm      = 7.0
	partnerAvgTaxiGperKm      = 180.0
	partnerAvgTransitGperKm   = 35.0
	partnerAvgSavedGperJourney = (partnerAvgTaxiGperKm - partnerAvgTransitGperKm) * partnerAvgDistanceKm
)

type PartnerCO2Stats struct {
	TotalJourneys   int64   `json:"total_journeys"`
	JourneysToday   int64   `json:"journeys_today"`
	JourneysMonth   int64   `json:"journeys_month"`
	CO2SavedKgTotal float64 `json:"co2_saved_kg_total"`
	CO2SavedKgToday float64 `json:"co2_saved_kg_today"`
	CO2SavedKgMonth float64 `json:"co2_saved_kg_month"`
	TreesEquivalent float64 `json:"trees_equivalent"`
	DailyTrend      []PartnerCO2Day `json:"daily_trend"`
}

type PartnerCO2Day struct {
	Date       string  `json:"date"`
	Journeys   int64   `json:"journeys"`
	CO2KgSaved float64 `json:"co2_kg_saved"`
}

func CO2Overview(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := context.Background()
		partnerID := c.Locals("partner_id").(string)

		var stats PartnerCO2Stats

		err := db.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%') AS total,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%' AND timestamp >= CURRENT_DATE) AS today,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%' AND timestamp >= date_trunc('month', NOW())) AS month
			FROM usage_log
			WHERE partner_id = $1
		`, partnerID).Scan(&stats.TotalJourneys, &stats.JourneysToday, &stats.JourneysMonth)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		stats.CO2SavedKgTotal = float64(stats.TotalJourneys) * partnerAvgSavedGperJourney / 1000.0
		stats.CO2SavedKgToday = float64(stats.JourneysToday) * partnerAvgSavedGperJourney / 1000.0
		stats.CO2SavedKgMonth = float64(stats.JourneysMonth) * partnerAvgSavedGperJourney / 1000.0
		stats.TreesEquivalent = stats.CO2SavedKgTotal / 21.0

		// Daily trend last 30 days
		rows, err := db.Query(ctx, `
			SELECT
				DATE(timestamp) AS day,
				COUNT(*) FILTER (WHERE endpoint LIKE '%route-search%') AS journeys
			FROM usage_log
			WHERE partner_id = $1
			  AND timestamp >= NOW() - INTERVAL '30 days'
			GROUP BY day
			ORDER BY day
		`, partnerID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var d PartnerCO2Day
				if err := rows.Scan(&d.Date, &d.Journeys); err == nil {
					d.CO2KgSaved = float64(d.Journeys) * partnerAvgSavedGperJourney / 1000.0
					stats.DailyTrend = append(stats.DailyTrend, d)
				}
			}
		}

		if stats.DailyTrend == nil {
			stats.DailyTrend = []PartnerCO2Day{}
		}

		return c.JSON(stats)
	}
}
