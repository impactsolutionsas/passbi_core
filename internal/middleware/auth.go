package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PartnerContext holds partner information for the request
type PartnerContext struct {
	PartnerID   string
	APIKeyID    string
	Tier        string
	Scopes      []string
	Email       string
	CompanyName string
}

// AuthMiddleware validates API key and loads partner information
func AuthMiddleware(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract API key from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{
				"error":   "missing_api_key",
				"message": "API key is required. Use Authorization: Bearer YOUR_API_KEY",
				"docs":    "https://docs.passbi.com/authentication",
			})
		}

		// Format: "Bearer pk_live_..."
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return c.Status(401).JSON(fiber.Map{
				"error":   "invalid_auth_format",
				"message": "Authorization header must be in format: Bearer YOUR_API_KEY",
				"example": "Authorization: Bearer pk_live_abc123...",
			})
		}

		apiKey := strings.TrimSpace(parts[1])

		// Validate basic format
		if !strings.HasPrefix(apiKey, "pk_") {
			return c.Status(401).JSON(fiber.Map{
				"error":   "invalid_api_key_format",
				"message": "API key must start with pk_",
			})
		}

		// Hash the key for database lookup
		hash := sha256.Sum256([]byte(apiKey))
		keyHash := hex.EncodeToString(hash[:])

		// Query database for API key and partner info
		ctx := context.Background()
		query := `
			SELECT
				ak.id,
				ak.partner_id,
				ak.scopes,
				ak.allowed_ips,
				p.tier,
				p.status,
				p.email,
				p.company,
				p.rate_limit_per_second,
				p.rate_limit_per_day,
				p.rate_limit_per_month
			FROM api_key ak
			JOIN partner p ON p.id = ak.partner_id
			WHERE ak.key_hash = $1
				AND ak.is_active = true
				AND p.status = 'active'
				AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
		`

		var (
			apiKeyID           string
			partnerID          string
			scopes             []string
			allowedIPs         []string
			tier               string
			status             string
			email              string
			company            string
			rateLimitPerSecond int
			rateLimitPerDay    int
			rateLimitPerMonth  int
		)

		err := db.QueryRow(ctx, query, keyHash).Scan(
			&apiKeyID,
			&partnerID,
			&scopes,
			&allowedIPs,
			&tier,
			&status,
			&email,
			&company,
			&rateLimitPerSecond,
			&rateLimitPerDay,
			&rateLimitPerMonth,
		)

		if err != nil {
			return c.Status(401).JSON(fiber.Map{
				"error":   "invalid_api_key",
				"message": "The provided API key is invalid, expired, or has been revoked",
			})
		}

		// Check IP whitelist if configured
		if len(allowedIPs) > 0 {
			clientIP := c.IP()
			allowed := false
			for _, allowedIP := range allowedIPs {
				if clientIP == allowedIP {
					allowed = true
					break
				}
			}
			if !allowed {
				return c.Status(403).JSON(fiber.Map{
					"error":   "ip_not_allowed",
					"message": "Your IP address is not authorized to use this API key",
					"ip":      clientIP,
				})
			}
		}

		// Update last_used_at asynchronously (non-blocking)
		go updateLastUsed(db, apiKeyID)

		// Store partner context in locals
		c.Locals("partner", &PartnerContext{
			PartnerID:   partnerID,
			APIKeyID:    apiKeyID,
			Tier:        tier,
			Scopes:      scopes,
			Email:       email,
			CompanyName: company,
		})

		// Store rate limits in locals for rate limiting middleware
		c.Locals("rate_limits", map[string]int{
			"per_second": rateLimitPerSecond,
			"per_day":    rateLimitPerDay,
			"per_month":  rateLimitPerMonth,
		})

		return c.Next()
	}
}

// updateLastUsed updates the last_used_at timestamp for an API key
func updateLastUsed(db *pgxpool.Pool, apiKeyID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE api_key
		SET last_used_at = NOW()
		WHERE id = $1
	`

	_, _ = db.Exec(ctx, query, apiKeyID)

	// Also update partner's last_active_at
	query = `
		UPDATE partner
		SET last_active_at = NOW()
		WHERE id = (SELECT partner_id FROM api_key WHERE id = $1)
	`
	_, _ = db.Exec(ctx, query, apiKeyID)
}

// RequireScope checks if the partner has a specific scope
func RequireScope(scope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		partner, ok := c.Locals("partner").(*PartnerContext)
		if !ok {
			return c.Status(401).JSON(fiber.Map{
				"error":   "unauthorized",
				"message": "Authentication required",
			})
		}

		// Check if partner has the required scope
		hasScope := false
		for _, s := range partner.Scopes {
			if s == scope || s == "*" {
				hasScope = true
				break
			}
		}

		if !hasScope {
			return c.Status(403).JSON(fiber.Map{
				"error":   "insufficient_permissions",
				"message": "Your API key does not have the required permissions",
				"required_scope": scope,
			})
		}

		return c.Next()
	}
}

// OptionalAuth is like AuthMiddleware but doesn't fail if no auth is provided
// Useful for endpoints that can work with or without authentication
func OptionalAuth(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			// No auth provided, continue without partner context
			return c.Next()
		}

		// Auth provided, validate it
		return AuthMiddleware(db)(c)
	}
}
