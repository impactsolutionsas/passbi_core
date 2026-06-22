package partner_portal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

type apiKeyRow struct {
	ID          string     `json:"id"`
	KeyPrefix   string     `json:"key_prefix"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Scopes      []string   `json:"scopes"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// ListAPIKeys GET /api/partner/api-keys
func ListAPIKeys(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT id, key_prefix, name, description, scopes, is_active, created_at, expires_at, last_used_at
			FROM api_key
			WHERE partner_id = $1
			ORDER BY created_at DESC
		`, p.PartnerID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		keys := []apiKeyRow{}
		for rows.Next() {
			var k apiKeyRow
			_ = rows.Scan(&k.ID, &k.KeyPrefix, &k.Name, &k.Description,
				&k.Scopes, &k.IsActive, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt)
			keys = append(keys, k)
		}
		return c.JSON(keys)
	}
}

type createKeyRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}

// CreateAPIKey POST /api/partner/api-keys
// Returns the raw key ONCE — never stored
func CreateAPIKey(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)

		var req createKeyRequest
		if err := c.BodyParser(&req); err != nil || req.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name is required"})
		}
		if len(req.Scopes) == 0 {
			req.Scopes = []string{"read:routes"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check max keys for tier
		var maxKeys int
		_ = db.QueryRow(ctx,
			`SELECT COALESCE((features->>'max_api_keys')::int, 5) FROM tier_config WHERE tier = (SELECT tier FROM partner WHERE id = $1)`,
			p.PartnerID,
		).Scan(&maxKeys)

		if maxKeys > 0 {
			var activeCount int
			_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM api_key WHERE partner_id = $1 AND is_active = true`, p.PartnerID).Scan(&activeCount)
			if activeCount >= maxKeys {
				return c.Status(429).JSON(fiber.Map{"error": "max_keys_reached", "limit": maxKeys})
			}
		}

		// Generate raw key
		raw := generateKey()
		prefix := raw[:20]
		hash := sha256Key(raw)

		var keyID string
		err := db.QueryRow(ctx, `
			INSERT INTO api_key (partner_id, key_hash, key_prefix, name, description, scopes, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, true)
			RETURNING id
		`, p.PartnerID, hash, prefix, req.Name, nullStr(req.Description), req.Scopes).Scan(&keyID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(201).JSON(fiber.Map{
			"id":         keyID,
			"key":        raw, // shown ONCE
			"key_prefix": prefix,
			"name":       req.Name,
			"scopes":     req.Scopes,
			"warning":    "Copiez cette clé maintenant — elle ne sera plus affichée.",
		})
	}
}

// RevokeAPIKey DELETE /api/partner/api-keys/:id
func RevokeAPIKey(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		keyID := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tag, err := db.Exec(ctx,
			`UPDATE api_key SET is_active = false WHERE id = $1 AND partner_id = $2`,
			keyID, p.PartnerID,
		)
		if err != nil || tag.RowsAffected() == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "key_not_found"})
		}
		return c.SendStatus(204)
	}
}

func generateKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "pk_live_" + hex.EncodeToString(b)
}

func sha256Key(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func init() {
	// silence unused import
	_ = fmt.Sprintf
}
