package partner_portal

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

type partnerProfile struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Company   string  `json:"company"`
	Tier      string  `json:"tier"`
	CreatedAt string  `json:"created_at"`
}

// GetProfile GET /api/partner/me/profile — full DB profile
func GetProfile(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var prof partnerProfile
		err := db.QueryRow(ctx, `
			SELECT id, email, COALESCE(contact_name, ''), COALESCE(company, ''), tier,
			       TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM partner WHERE id = $1
		`, p.PartnerID).Scan(&prof.ID, &prof.Email, &prof.Name, &prof.Company, &prof.Tier, &prof.CreatedAt)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(prof)
	}
}

type updateProfileRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// UpdateProfile PATCH /api/partner/me/profile
func UpdateProfile(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)

		var req updateProfileRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Name == nil && req.Email == nil {
			return c.Status(400).JSON(fiber.Map{"error": "no fields to update"})
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name cannot be empty"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		setClauses := []string{}
		args := []any{}
		i := 1
		if req.Name != nil {
			setClauses = append(setClauses, "contact_name = $"+strconv.Itoa(i))
			args = append(args, strings.TrimSpace(*req.Name))
			i++
		}
		if req.Email != nil {
			setClauses = append(setClauses, "email = $"+strconv.Itoa(i))
			args = append(args, *req.Email)
			i++
		}

		args = append(args, p.PartnerID)
		_, err := db.Exec(ctx,
			"UPDATE partner SET "+strings.Join(setClauses, ", ")+" WHERE id = $"+strconv.Itoa(i),
			args...,
		)
		if err != nil {
			if strings.Contains(err.Error(), "unique") {
				return c.Status(409).JSON(fiber.Map{"error": "email_already_exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		updated := fiber.Map{"id": p.PartnerID}
		if req.Name != nil {
			updated["name"] = strings.TrimSpace(*req.Name)
		}
		if req.Email != nil {
			updated["email"] = *req.Email
		}
		return c.JSON(updated)
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword PATCH /api/partner/me/password
func ChangePassword(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := c.Locals("portal_partner").(*middleware.PartnerPortalContext)

		var req changePasswordRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.CurrentPassword == "" || req.NewPassword == "" {
			return c.Status(400).JSON(fiber.Map{"error": "current_password and new_password are required"})
		}
		if len(req.NewPassword) < 8 {
			return c.Status(400).JSON(fiber.Map{"error": "new_password must be at least 8 characters"})
		}
		if req.CurrentPassword == req.NewPassword {
			return c.Status(400).JSON(fiber.Map{"error": "new_password must differ from current_password"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var currentHash *string
		if err := db.QueryRow(ctx,
			`SELECT password_hash FROM partner WHERE id = $1`, p.PartnerID,
		).Scan(&currentHash); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to fetch partner"})
		}

		if currentHash == nil {
			return c.Status(403).JSON(fiber.Map{"error": "portal_access_not_enabled"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(*currentHash), []byte(req.CurrentPassword)); err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "current_password is incorrect"})
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}

		if _, err = db.Exec(ctx,
			`UPDATE partner SET password_hash = $1 WHERE id = $2`, string(newHash), p.PartnerID,
		); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "password changed successfully"})
	}
}
