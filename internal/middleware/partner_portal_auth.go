package middleware

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type PartnerPortalContext struct {
	PartnerID string
	Email     string
	Name      string
	Company   string
	Tier      string
}

// PartnerPortalAuthMiddleware validates a JWT issued to a partner for the self-service portal
func PartnerPortalAuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "missing_token"})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return c.Status(401).JSON(fiber.Map{"error": "invalid_auth_format"})
		}

		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "changeme-jwt-secret"
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			return c.Status(401).JSON(fiber.Map{"error": "invalid_token"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "invalid_claims"})
		}

		if claims["aud"] != "partner_portal" {
			return c.Status(403).JSON(fiber.Map{"error": "wrong_audience"})
		}

		c.Locals("portal_partner", &PartnerPortalContext{
			PartnerID: claims["sub"].(string),
			Email:     claims["email"].(string),
			Name:      claims["name"].(string),
			Company:   claims["company"].(string),
			Tier:      claims["tier"].(string),
		})

		return c.Next()
	}
}
