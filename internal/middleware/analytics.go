package middleware

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequestLog holds information about an API request for logging
type RequestLog struct {
	PartnerID      string
	APIKeyID       string
	Endpoint       string
	Method         string
	ResponseTimeMs int
	ResponseStatus int
	FromLocation   *Location
	ToLocation     *Location
	CacheHit       bool
	IPAddress      string
	UserAgent      string
	Timestamp      time.Time
}

// Location represents a geographic coordinate
type Location struct {
	Lat float64
	Lon float64
}

// AnalyticsMiddleware logs all API requests for analytics and billing
func AnalyticsMiddleware(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Record start time
		start := time.Now()

		// Process the request
		err := c.Next()

		// Calculate response time
		responseTime := time.Since(start)

		// Get partner context
		partner, ok := c.Locals("partner").(*PartnerContext)
		if !ok {
			// No partner context, skip logging (shouldn't happen after auth)
			return err
		}

		// Check if this was a cache hit
		cacheHit := false
		if val := c.Locals("cache_hit"); val != nil {
			cacheHit = val.(bool)
		}

		// Extract location data if available (for route-search endpoint)
		var fromLoc, toLoc *Location
		if c.Path() == "/v2/route-search" {
			if from := c.Query("from"); from != "" {
				fromLoc = parseLocationFromQuery(from)
			}
			if to := c.Query("to"); to != "" {
				toLoc = parseLocationFromQuery(to)
			}
		}

		// Create request log
		requestLog := &RequestLog{
			PartnerID:      partner.PartnerID,
			APIKeyID:       partner.APIKeyID,
			Endpoint:       c.Path(),
			Method:         c.Method(),
			ResponseTimeMs: int(responseTime.Milliseconds()),
			ResponseStatus: c.Response().StatusCode(),
			FromLocation:   fromLoc,
			ToLocation:     toLoc,
			CacheHit:       cacheHit,
			IPAddress:      c.IP(),
			UserAgent:      c.Get("User-Agent"),
			Timestamp:      time.Now(),
		}

		// Log asynchronously (non-blocking)
		go logRequest(db, requestLog)

		// Add custom response headers for debugging
		c.Set("X-Response-Time", responseTime.String())
		c.Set("X-Cache-Hit", boolToString(cacheHit))

		return err
	}
}

// logRequest logs a request to the database
func logRequest(db *pgxpool.Pool, reqLog *RequestLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO usage_log (
			partner_id,
			api_key_id,
			endpoint,
			method,
			response_time_ms,
			response_status,
			from_location,
			to_location,
			cache_hit,
			ip_address,
			user_agent,
			timestamp
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	var fromPoint, toPoint interface{}
	if reqLog.FromLocation != nil {
		fromPoint = &reqLog.FromLocation
	}
	if reqLog.ToLocation != nil {
		toPoint = &reqLog.ToLocation
	}

	_, err := db.Exec(ctx, query,
		reqLog.PartnerID,
		reqLog.APIKeyID,
		reqLog.Endpoint,
		reqLog.Method,
		reqLog.ResponseTimeMs,
		reqLog.ResponseStatus,
		fromPoint,
		toPoint,
		reqLog.CacheHit,
		reqLog.IPAddress,
		reqLog.UserAgent,
		reqLog.Timestamp,
	)

	if err != nil {
		log.Println("Failed to log request:", err)
	}

	// Update quota usage
	updateQuotaUsage(db, reqLog.PartnerID, reqLog.ResponseStatus >= 200 && reqLog.ResponseStatus < 300)
}

// updateQuotaUsage updates daily and monthly quota counters
func updateQuotaUsage(db *pgxpool.Pool, partnerID string, success bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// Update daily quota
	queryDaily := `
		INSERT INTO quota_usage (
			partner_id,
			period_type,
			period_start,
			period_end,
			requests_count,
			successful_requests,
			failed_requests
		)
		VALUES ($1, 'daily', $2, $2, 1, $3, $4)
		ON CONFLICT (partner_id, period_type, period_start)
		DO UPDATE SET
			requests_count = quota_usage.requests_count + 1,
			successful_requests = quota_usage.successful_requests + $3,
			failed_requests = quota_usage.failed_requests + $4,
			updated_at = NOW()
	`

	successCount := 0
	failCount := 0
	if success {
		successCount = 1
	} else {
		failCount = 1
	}

	_, err := db.Exec(ctx, queryDaily,
		partnerID,
		now.Format("2006-01-02"),
		successCount,
		failCount,
	)

	if err != nil {
		log.Println("Failed to update daily quota:", err)
	}

	// Update monthly quota
	firstDayOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastDayOfMonth := firstDayOfMonth.AddDate(0, 1, -1)

	queryMonthly := `
		INSERT INTO quota_usage (
			partner_id,
			period_type,
			period_start,
			period_end,
			requests_count,
			successful_requests,
			failed_requests
		)
		VALUES ($1, 'monthly', $2, $3, 1, $4, $5)
		ON CONFLICT (partner_id, period_type, period_start)
		DO UPDATE SET
			requests_count = quota_usage.requests_count + 1,
			successful_requests = quota_usage.successful_requests + $4,
			failed_requests = quota_usage.failed_requests + $5,
			updated_at = NOW()
	`

	_, err = db.Exec(ctx, queryMonthly,
		partnerID,
		firstDayOfMonth.Format("2006-01-02"),
		lastDayOfMonth.Format("2006-01-02"),
		successCount,
		failCount,
	)

	if err != nil {
		log.Println("Failed to update monthly quota:", err)
	}
}

// parseLocationFromQuery parses "lat,lon" string into Location
func parseLocationFromQuery(query string) *Location {
	var lat, lon float64
	_, err := fmt.Sscanf(query, "%f,%f", &lat, &lon)
	if err != nil {
		return nil
	}
	return &Location{Lat: lat, Lon: lon}
}

// boolToString converts bool to string for headers
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// GetPartnerAnalytics retrieves analytics data for a partner
func GetPartnerAnalytics(db *pgxpool.Pool, partnerID string, startDate, endDate time.Time) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT
			DATE(timestamp) as date,
			COUNT(*) as total_requests,
			COUNT(*) FILTER (WHERE response_status >= 200 AND response_status < 300) as successful,
			COUNT(*) FILTER (WHERE response_status >= 400) as failed,
			AVG(response_time_ms) as avg_response_time,
			MAX(response_time_ms) as max_response_time,
			MIN(response_time_ms) as min_response_time,
			COUNT(*) FILTER (WHERE cache_hit = true) as cache_hits,
			COUNT(DISTINCT ip_address) as unique_ips
		FROM usage_log
		WHERE partner_id = $1
			AND timestamp >= $2
			AND timestamp <= $3
		GROUP BY DATE(timestamp)
		ORDER BY date DESC
	`

	rows, err := db.Query(ctx, query, partnerID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var (
			date            time.Time
			total           int64
			successful      int64
			failed          int64
			avgResponse     float64
			maxResponse     int
			minResponse     int
			cacheHits       int64
			uniqueIPs       int64
		)

		err := rows.Scan(&date, &total, &successful, &failed, &avgResponse, &maxResponse, &minResponse, &cacheHits, &uniqueIPs)
		if err != nil {
			continue
		}

		stats = append(stats, map[string]interface{}{
			"date":             date.Format("2006-01-02"),
			"total_requests":   total,
			"successful":       successful,
			"failed":           failed,
			"avg_response_ms":  avgResponse,
			"max_response_ms":  maxResponse,
			"min_response_ms":  minResponse,
			"cache_hits":       cacheHits,
			"cache_hit_rate":   float64(cacheHits) / float64(total) * 100,
			"unique_ips":       uniqueIPs,
		})
	}

	return map[string]interface{}{
		"stats": stats,
		"summary": calculateSummary(stats),
	}, nil
}

// calculateSummary calculates aggregate statistics
func calculateSummary(stats []map[string]interface{}) map[string]interface{} {
	if len(stats) == 0 {
		return map[string]interface{}{}
	}

	var totalRequests, totalSuccessful, totalFailed int64
	var totalCacheHits int64
	var sumAvgResponse float64

	for _, stat := range stats {
		totalRequests += stat["total_requests"].(int64)
		totalSuccessful += stat["successful"].(int64)
		totalFailed += stat["failed"].(int64)
		totalCacheHits += stat["cache_hits"].(int64)
		sumAvgResponse += stat["avg_response_ms"].(float64)
	}

	return map[string]interface{}{
		"total_requests":      totalRequests,
		"total_successful":    totalSuccessful,
		"total_failed":        totalFailed,
		"success_rate":        float64(totalSuccessful) / float64(totalRequests) * 100,
		"total_cache_hits":    totalCacheHits,
		"overall_cache_rate":  float64(totalCacheHits) / float64(totalRequests) * 100,
		"avg_response_ms":     sumAvgResponse / float64(len(stats)),
		"days_analyzed":       len(stats),
	}
}
