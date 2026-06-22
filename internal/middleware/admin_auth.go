package middleware

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type AdminContext struct {
	AdminID string
	Email   string
	Name    string
	Role    string
}

// AdminAuthMiddleware validates a JWT issued to an admin user
func AdminAuthMiddleware() fiber.Handler {
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

		if claims["aud"] != "admin" {
			return c.Status(403).JSON(fiber.Map{"error": "wrong_audience"})
		}

		c.Locals("admin", &AdminContext{
			AdminID: claims["sub"].(string),
			Email:   claims["email"].(string),
			Name:    claims["name"].(string),
			Role:    claims["role"].(string),
		})

		return c.Next()
	}
}

// RequireAdminRole ensures the admin has a specific role.
// superadmin always passes.
func RequireAdminRole(role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		admin, ok := c.Locals("admin").(*AdminContext)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if admin.Role == "superadmin" {
			return c.Next()
		}
		if admin.Role != role {
			return c.Status(403).JSON(fiber.Map{"error": "insufficient_role"})
		}
		return c.Next()
	}
}

// RequireAnyRole grants access if the admin has one of the allowed roles.
// superadmin always passes regardless of the list.
func RequireAnyRole(roles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		admin, ok := c.Locals("admin").(*AdminContext)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if admin.Role == "superadmin" {
			return c.Next()
		}
		if _, ok := allowed[admin.Role]; !ok {
			return c.Status(403).JSON(fiber.Map{"error": "insufficient_role"})
		}
		return c.Next()
	}
}
