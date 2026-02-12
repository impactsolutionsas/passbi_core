package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RateLimitMiddleware implements multi-level rate limiting
// It checks limits per second, per day, and per month
func RateLimitMiddleware(rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get partner context from auth middleware
		partner, ok := c.Locals("partner").(*PartnerContext)
		if !ok {
			// If no partner context, skip rate limiting (shouldn't happen after auth middleware)
			return c.Next()
		}

		// Get rate limits from context
		rateLimits, ok := c.Locals("rate_limits").(map[string]int)
		if !ok {
			// If no rate limits configured, use defaults
			rateLimits = map[string]int{
				"per_second": 10,
				"per_day":    10000,
				"per_month":  300000,
			}
		}

		ctx := context.Background()
		now := time.Now()

		// Generate Redis keys for different time periods
		keySecond := fmt.Sprintf("rl:partner:%s:second:%d", partner.PartnerID, now.Unix())
		keyDay := fmt.Sprintf("rl:partner:%s:day:%s", partner.PartnerID, now.Format("2006-01-02"))
		keyMonth := fmt.Sprintf("rl:partner:%s:month:%s", partner.PartnerID, now.Format("2006-01"))

		// Check per-second rate limit
		if rateLimits["per_second"] > 0 {
			countSecond, err := rdb.Incr(ctx, keySecond).Result()
			if err == nil {
				// Set expiration for per-second counter
				rdb.Expire(ctx, keySecond, 2*time.Second)

				if countSecond > int64(rateLimits["per_second"]) {
					// Add rate limit headers
					c.Set("X-RateLimit-Limit-Second", strconv.Itoa(rateLimits["per_second"]))
					c.Set("X-RateLimit-Remaining-Second", "0")
					c.Set("X-RateLimit-Reset-Second", strconv.FormatInt(now.Unix()+1, 10))
					c.Set("Retry-After", "1")

					return c.Status(429).JSON(fiber.Map{
						"error":       "rate_limit_exceeded",
						"message":     "Too many requests per second",
						"limit_type":  "per_second",
						"limit":       rateLimits["per_second"],
						"retry_after": 1,
					})
				}
			}
		}

		// Check per-day rate limit
		if rateLimits["per_day"] > 0 {
			countDay, err := rdb.Incr(ctx, keyDay).Result()
			if err == nil {
				// Set expiration for per-day counter
				rdb.Expire(ctx, keyDay, 25*time.Hour) // 25 hours to handle timezone differences

				if countDay > int64(rateLimits["per_day"]) {
					// Calculate seconds until midnight
					tomorrow := now.AddDate(0, 0, 1)
					midnight := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
					retryAfter := int64(midnight.Sub(now).Seconds())

					c.Set("X-RateLimit-Limit-Day", strconv.Itoa(rateLimits["per_day"]))
					c.Set("X-RateLimit-Remaining-Day", "0")
					c.Set("X-RateLimit-Reset-Day", strconv.FormatInt(midnight.Unix(), 10))
					c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))

					return c.Status(429).JSON(fiber.Map{
						"error":       "daily_quota_exceeded",
						"message":     "Daily quota exceeded",
						"limit_type":  "per_day",
						"limit":       rateLimits["per_day"],
						"used":        countDay,
						"retry_after": retryAfter,
						"reset_at":    midnight.Format(time.RFC3339),
					})
				}

				// Set remaining count header
				c.Set("X-RateLimit-Remaining-Day", strconv.FormatInt(int64(rateLimits["per_day"])-countDay, 10))
			}
		}

		// Check per-month rate limit
		if rateLimits["per_month"] > 0 {
			countMonth, err := rdb.Incr(ctx, keyMonth).Result()
			if err == nil {
				// Set expiration for per-month counter
				rdb.Expire(ctx, keyMonth, 32*24*time.Hour) // 32 days

				if countMonth > int64(rateLimits["per_month"]) {
					// Calculate seconds until next month
					firstDayNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
					retryAfter := int64(firstDayNextMonth.Sub(now).Seconds())

					c.Set("X-RateLimit-Limit-Month", strconv.Itoa(rateLimits["per_month"]))
					c.Set("X-RateLimit-Remaining-Month", "0")
					c.Set("X-RateLimit-Reset-Month", strconv.FormatInt(firstDayNextMonth.Unix(), 10))
					c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))

					return c.Status(429).JSON(fiber.Map{
						"error":       "monthly_quota_exceeded",
						"message":     "Monthly quota exceeded",
						"limit_type":  "per_month",
						"limit":       rateLimits["per_month"],
						"used":        countMonth,
						"retry_after": retryAfter,
						"reset_at":    firstDayNextMonth.Format(time.RFC3339),
					})
				}

				// Set remaining count header
				c.Set("X-RateLimit-Remaining-Month", strconv.FormatInt(int64(rateLimits["per_month"])-countMonth, 10))
			}
		}

		// Add rate limit headers to response
		c.Set("X-RateLimit-Limit-Second", strconv.Itoa(rateLimits["per_second"]))
		c.Set("X-RateLimit-Limit-Day", strconv.Itoa(rateLimits["per_day"]))
		c.Set("X-RateLimit-Limit-Month", strconv.Itoa(rateLimits["per_month"]))

		// Store counts in locals for analytics middleware
		c.Locals("rate_limit_counts", map[string]int64{
			"second": getCurrentCount(ctx, rdb, keySecond),
			"day":    getCurrentCount(ctx, rdb, keyDay),
			"month":  getCurrentCount(ctx, rdb, keyMonth),
		})

		return c.Next()
	}
}

// getCurrentCount gets the current count from Redis
func getCurrentCount(ctx context.Context, rdb *redis.Client, key string) int64 {
	val, err := rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

// ResetRateLimit resets rate limits for a partner (admin function)
func ResetRateLimit(rdb *redis.Client, partnerID string, period string) error {
	ctx := context.Background()
	now := time.Now()

	var key string
	switch period {
	case "second":
		key = fmt.Sprintf("rl:partner:%s:second:%d", partnerID, now.Unix())
	case "day":
		key = fmt.Sprintf("rl:partner:%s:day:%s", partnerID, now.Format("2006-01-02"))
	case "month":
		key = fmt.Sprintf("rl:partner:%s:month:%s", partnerID, now.Format("2006-01"))
	default:
		return fmt.Errorf("invalid period: %s", period)
	}

	return rdb.Del(ctx, key).Err()
}

// GetRateLimitStatus gets current rate limit status for a partner
func GetRateLimitStatus(rdb *redis.Client, partnerID string, rateLimits map[string]int) map[string]interface{} {
	ctx := context.Background()
	now := time.Now()

	keySecond := fmt.Sprintf("rl:partner:%s:second:%d", partnerID, now.Unix())
	keyDay := fmt.Sprintf("rl:partner:%s:day:%s", partnerID, now.Format("2006-01-02"))
	keyMonth := fmt.Sprintf("rl:partner:%s:month:%s", partnerID, now.Format("2006-01"))

	countSecond := getCurrentCount(ctx, rdb, keySecond)
	countDay := getCurrentCount(ctx, rdb, keyDay)
	countMonth := getCurrentCount(ctx, rdb, keyMonth)

	return map[string]interface{}{
		"second": map[string]interface{}{
			"limit":     rateLimits["per_second"],
			"used":      countSecond,
			"remaining": maxInt64(0, int64(rateLimits["per_second"])-countSecond),
		},
		"day": map[string]interface{}{
			"limit":     rateLimits["per_day"],
			"used":      countDay,
			"remaining": maxInt64(0, int64(rateLimits["per_day"])-countDay),
		},
		"month": map[string]interface{}{
			"limit":     rateLimits["per_month"],
			"used":      countMonth,
			"remaining": maxInt64(0, int64(rateLimits["per_month"])-countMonth),
		},
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
