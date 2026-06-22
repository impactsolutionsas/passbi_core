package admin

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/audit"
	"github.com/passbi/passbi_core/internal/middleware"
)

type apiKeyRow struct {
	ID          string     `json:"id"`
	PartnerID   string     `json:"partner_id"`
	KeyPrefix   string     `json:"key_prefix"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Scopes      []string   `json:"scopes"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// ListPartnerAPIKeys GET /api/admin/partners/:id/api-keys
func ListPartnerAPIKeys(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		partnerID := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT id, partner_id, key_prefix, name, description, scopes, is_active, created_at, expires_at, last_used_at
			FROM api_key
			WHERE partner_id = $1
			ORDER BY created_at DESC
		`, partnerID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		keys := []apiKeyRow{}
		for rows.Next() {
			var k apiKeyRow
			if err := rows.Scan(
				&k.ID, &k.PartnerID, &k.KeyPrefix, &k.Name, &k.Description,
				&k.Scopes, &k.IsActive, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt,
			); err != nil {
				continue
			}
			keys = append(keys, k)
		}

		return c.JSON(keys)
	}
}

// RevokeAPIKey DELETE /api/admin/api-keys/:keyId
func RevokeAPIKey(db *pgxpool.Pool, al *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		keyID := c.Params("keyId")
		actor, _ := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Capture key info before revocation
		var keyName, partnerID, keyPrefix string
		_ = db.QueryRow(ctx,
			`SELECT name, partner_id, key_prefix FROM api_key WHERE id = $1`, keyID,
		).Scan(&keyName, &partnerID, &keyPrefix)

		tag, err := db.Exec(ctx, `UPDATE api_key SET is_active = false WHERE id = $1`, keyID)
		if err != nil || tag.RowsAffected() == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "api_key_not_found"})
		}

		al.LogAsync(ctx, audit.Entry{
			EventType:    audit.EventAPIKeyRevoked,
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "api_key",
			ResourceID:   keyID,
			Action:       audit.ActionRevoke,
			RequestID:    reqID,
			BeforeState: map[string]any{
				"key_prefix": keyPrefix,
				"name":       keyName,
				"partner_id": partnerID,
				"is_active":  true,
			},
			AfterState: map[string]any{"is_active": false},
		})

		return c.SendStatus(204)
	}
}
