package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
	"github.com/redis/go-redis/v9"
)

// Partner represents partner account information
type Partner struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Company            string     `json:"company,omitempty"`
	Status             string     `json:"status"`
	Tier               string     `json:"tier"`
	RateLimitPerSecond int        `json:"rate_limit_per_second"`
	RateLimitPerDay    int        `json:"rate_limit_per_day"`
	RateLimitPerMonth  int        `json:"rate_limit_per_month"`
	CreatedAt          time.Time  `json:"created_at"`
	LastActiveAt       *time.Time `json:"last_active_at,omitempty"`
}

// APIKey represents an API key (sanitized for display)
type APIKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Description string     `json:"description,omitempty"`
	Scopes      []string   `json:"scopes"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// UsageStat represents usage statistics
type UsageStat struct {
	Date            string  `json:"date"`
	TotalRequests   int64   `json:"total_requests"`
	Successful      int64   `json:"successful"`
	Failed          int64   `json:"failed"`
	AvgResponseTime float64 `json:"avg_response_time_ms"`
	CacheHits       int64   `json:"cache_hits"`
	CacheHitRate    float64 `json:"cache_hit_rate"`
}

// GetPartnerInfo returns the authenticated partner's information
func GetPartnerInfo(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)

	ctx := context.Background()
	query := `
		SELECT
			id, name, email, COALESCE(company, ''), status, tier,
			rate_limit_per_second, rate_limit_per_day, rate_limit_per_month,
			created_at, last_active_at
		FROM partner
		WHERE id = $1
	`

	var p Partner
	err := pool.QueryRow(ctx, query, partner.PartnerID).Scan(
		&p.ID, &p.Name, &p.Email, &p.Company, &p.Status, &p.Tier,
		&p.RateLimitPerSecond, &p.RateLimitPerDay, &p.RateLimitPerMonth,
		&p.CreatedAt, &p.LastActiveAt,
	)

	if err != nil {
		log.Printf("Failed to get partner info: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": "Failed to retrieve partner information",
		})
	}

	return c.JSON(p)
}

// GetAPIKeys returns all API keys for the authenticated partner
func GetAPIKeys(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)

	ctx := context.Background()
	query := `
		SELECT
			id, name, key_prefix, COALESCE(description, ''), scopes,
			is_active, created_at, expires_at, last_used_at
		FROM api_key
		WHERE partner_id = $1
		ORDER BY created_at DESC
	`

	rows, err := pool.Query(ctx, query, partner.PartnerID)
	if err != nil {
		log.Printf("Failed to get API keys: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": "Failed to retrieve API keys",
		})
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		err := rows.Scan(
			&k.ID, &k.Name, &k.KeyPrefix, &k.Description, &k.Scopes,
			&k.IsActive, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt,
		)
		if err != nil {
			log.Printf("Failed to scan API key: %v", err)
			continue
		}
		keys = append(keys, k)
	}

	if keys == nil {
		keys = []APIKey{}
	}

	return c.JSON(fiber.Map{
		"api_keys": keys,
		"total":    len(keys),
	})
}

// CreateAPIKeyRequest represents the request body for creating an API key
type CreateAPIKeyRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// CreateAPIKey creates a new API key for the authenticated partner
func CreateAPIKey(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)

	var req CreateAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "validation_error",
			"message": "API key name is required",
		})
	}

	if len(req.Scopes) == 0 {
		req.Scopes = []string{"read:routes"} // Default scope
	}

	// Check if partner has reached their API key limit
	ctx := context.Background()

	// Get tier config
	var maxKeys int
	tierQuery := `
		SELECT (features->>'max_api_keys')::int
		FROM tier_config
		WHERE tier = (SELECT tier FROM partner WHERE id = $1)
	`
	err := pool.QueryRow(ctx, tierQuery, partner.PartnerID).Scan(&maxKeys)
	if err == nil && maxKeys > 0 {
		// Check current count
		var currentCount int
		countQuery := `SELECT COUNT(*) FROM api_key WHERE partner_id = $1 AND is_active = true`
		pool.QueryRow(ctx, countQuery, partner.PartnerID).Scan(&currentCount)

		if currentCount >= maxKeys {
			return c.Status(400).JSON(fiber.Map{
				"error":   "limit_exceeded",
				"message": fmt.Sprintf("You have reached the maximum number of API keys (%d) for your plan", maxKeys),
			})
		}
	}

	// Generate a new API key
	apiKey, keyHash, keyPrefix := generateAPIKey("live")

	// Insert into database
	query := `
		INSERT INTO api_key (
			partner_id, key_hash, key_prefix, name, description, scopes, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	var keyID string
	var createdAt time.Time
	err = pool.QueryRow(ctx, query,
		partner.PartnerID, keyHash, keyPrefix, req.Name, req.Description, req.Scopes, req.ExpiresAt,
	).Scan(&keyID, &createdAt)

	if err != nil {
		log.Printf("Failed to create API key: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": "Failed to create API key",
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":         keyID,
		"api_key":    apiKey, // Show ONLY ONCE
		"key_prefix": keyPrefix,
		"name":       req.Name,
		"scopes":     req.Scopes,
		"created_at": createdAt,
		"warning":    "⚠️ Save this key now. You won't be able to see it again!",
	})
}

// RevokeAPIKey revokes (deactivates) an API key
func RevokeAPIKey(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)

	keyID := c.Params("id")
	if keyID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "invalid_request",
			"message": "API key ID is required",
		})
	}

	ctx := context.Background()
	query := `
		UPDATE api_key
		SET is_active = false
		WHERE id = $1 AND partner_id = $2
		RETURNING id
	`

	var id string
	err := pool.QueryRow(ctx, query, keyID, partner.PartnerID).Scan(&id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "not_found",
			"message": "API key not found or already revoked",
		})
	}

	return c.JSON(fiber.Map{
		"message": "API key revoked successfully",
		"id":      id,
	})
}

// GetUsageStats returns usage statistics for the authenticated partner
func GetUsageStats(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)

	// Parse query parameters
	daysStr := c.Query("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 90 {
		days = 30
	}

	ctx := context.Background()
	query := `
		SELECT
			DATE(timestamp) as date,
			COUNT(*) as total_requests,
			COUNT(*) FILTER (WHERE response_status >= 200 AND response_status < 300) as successful,
			COUNT(*) FILTER (WHERE response_status >= 400) as failed,
			AVG(response_time_ms) as avg_response_time,
			COUNT(*) FILTER (WHERE cache_hit = true) as cache_hits
		FROM usage_log
		WHERE partner_id = $1
			AND timestamp >= NOW() - INTERVAL '1 day' * $2
		GROUP BY DATE(timestamp)
		ORDER BY date DESC
	`

	rows, err := pool.Query(ctx, query, partner.PartnerID, days)
	if err != nil {
		log.Printf("Failed to get usage stats: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "internal_server_error",
			"message": "Failed to retrieve usage statistics",
		})
	}
	defer rows.Close()

	var stats []UsageStat
	for rows.Next() {
		var s UsageStat
		var date time.Time
		err := rows.Scan(&date, &s.TotalRequests, &s.Successful, &s.Failed, &s.AvgResponseTime, &s.CacheHits)
		if err != nil {
			log.Printf("Failed to scan usage stat: %v", err)
			continue
		}
		s.Date = date.Format("2006-01-02")
		if s.TotalRequests > 0 {
			s.CacheHitRate = float64(s.CacheHits) / float64(s.TotalRequests) * 100
		}
		stats = append(stats, s)
	}

	if stats == nil {
		stats = []UsageStat{}
	}

	return c.JSON(fiber.Map{
		"stats": stats,
		"period": fiber.Map{
			"days": days,
			"from": time.Now().AddDate(0, 0, -days).Format("2006-01-02"),
			"to":   time.Now().Format("2006-01-02"),
		},
	})
}

// GetQuotaUsage returns current quota usage for the authenticated partner
func GetQuotaUsage(c *fiber.Ctx) error {
	partner := c.Locals("partner").(*middleware.PartnerContext)
	pool := c.Locals("db").(*pgxpool.Pool)
	rdb := c.Locals("redis").(*redis.Client)

	ctx := context.Background()

	// Get rate limits
	rateLimits := c.Locals("rate_limits").(map[string]int)

	// Get current usage from Redis
	rateLimitStatus := middleware.GetRateLimitStatus(rdb, partner.PartnerID, rateLimits)

	// Get daily quota from database
	today := time.Now().Format("2006-01-02")
	dailyQuery := `
		SELECT requests_count, successful_requests, failed_requests
		FROM quota_usage
		WHERE partner_id = $1
			AND period_type = 'daily'
			AND period_start = $2
	`

	var dailyRequests, dailySuccessful, dailyFailed int64
	err := pool.QueryRow(ctx, dailyQuery, partner.PartnerID, today).Scan(&dailyRequests, &dailySuccessful, &dailyFailed)
	if err != nil {
		// No data for today yet
		dailyRequests = 0
		dailySuccessful = 0
		dailyFailed = 0
	}

	// Get monthly quota from database
	firstDayOfMonth := time.Now().Format("2006-01") + "-01"
	monthlyQuery := `
		SELECT requests_count, successful_requests, failed_requests
		FROM quota_usage
		WHERE partner_id = $1
			AND period_type = 'monthly'
			AND period_start = $2
	`

	var monthlyRequests, monthlySuccessful, monthlyFailed int64
	err = pool.QueryRow(ctx, monthlyQuery, partner.PartnerID, firstDayOfMonth).Scan(&monthlyRequests, &monthlySuccessful, &monthlyFailed)
	if err != nil {
		// No data for this month yet
		monthlyRequests = 0
		monthlySuccessful = 0
		monthlyFailed = 0
	}

	return c.JSON(fiber.Map{
		"rate_limits": rateLimitStatus,
		"daily": fiber.Map{
			"requests":   dailyRequests,
			"successful": dailySuccessful,
			"failed":     dailyFailed,
			"limit":      rateLimits["per_day"],
			"remaining":  maxInt64(0, int64(rateLimits["per_day"])-dailyRequests),
		},
		"monthly": fiber.Map{
			"requests":   monthlyRequests,
			"successful": monthlySuccessful,
			"failed":     monthlyFailed,
			"limit":      rateLimits["per_month"],
			"remaining":  maxInt64(0, int64(rateLimits["per_month"])-monthlyRequests),
		},
		"tier": partner.Tier,
	})
}

// generateAPIKey generates a new API key with hash and prefix
func generateAPIKey(env string) (key, hash, prefix string) {
	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomStr := hex.EncodeToString(randomBytes)

	// Generate checksum (first 2 bytes of hash)
	checksumBytes := sha256.Sum256([]byte(randomStr))
	checksum := hex.EncodeToString(checksumBytes[:2])

	// Construct the key
	key = fmt.Sprintf("pk_%s_%s_%s", env, randomStr, checksum)

	// Hash for storage
	hashBytes := sha256.Sum256([]byte(key))
	hash = hex.EncodeToString(hashBytes[:])

	// Prefix for display (first 12 chars after pk_env_)
	prefix = fmt.Sprintf("pk_%s_%s...", env, randomStr[:8])

	return
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
