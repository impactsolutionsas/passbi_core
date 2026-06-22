package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/audit"
	"github.com/passbi/passbi_core/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

// GetMyProfile GET /api/admin/me/profile — returns full DB profile (not just JWT claims)
func GetMyProfile(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var u adminUserRow
		err := db.QueryRow(ctx, `
			SELECT id, email, name, role, is_active, created_at, updated_at, last_login_at
			FROM admin_user WHERE id = $1
		`, actor.AdminID).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsActive,
			&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "user_not_found"})
		}
		return c.JSON(u)
	}
}

type updateProfileRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// UpdateProfile PATCH /api/admin/me/profile
func UpdateProfile(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

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

		var beforeName, beforeEmail string
		_ = db.QueryRow(ctx, `SELECT name, email FROM admin_user WHERE id = $1`, actor.AdminID).
			Scan(&beforeName, &beforeEmail)

		setClauses := []string{}
		args := []any{}
		i := 1
		if req.Name != nil {
			setClauses = append(setClauses, "name = $"+strconv.Itoa(i))
			args = append(args, strings.TrimSpace(*req.Name))
			i++
		}
		if req.Email != nil {
			setClauses = append(setClauses, "email = $"+strconv.Itoa(i))
			args = append(args, *req.Email)
			i++
		}

		args = append(args, actor.AdminID)
		_, err := db.Exec(ctx,
			"UPDATE admin_user SET "+strings.Join(setClauses, ", ")+" WHERE id = $"+strconv.Itoa(i),
			args...,
		)
		if err != nil {
			if isDuplicateKey(err) {
				return c.Status(409).JSON(fiber.Map{"error": "email_already_exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.profile_updated",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   actor.AdminID,
			Action:       audit.ActionUpdate,
			BeforeState:  map[string]any{"name": beforeName, "email": beforeEmail},
			AfterState:   req,
			RequestID:    reqID,
		})

		updated := fiber.Map{"id": actor.AdminID}
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

// ChangePassword PATCH /api/admin/me/password
func ChangePassword(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

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

		var currentHash string
		if err := db.QueryRow(ctx, `SELECT password_hash FROM admin_user WHERE id = $1`, actor.AdminID).
			Scan(&currentHash); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to fetch user"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType: "admin_user.password_change_failed",
				ActorType: audit.ActorAdmin,
				ActorID:   actor.AdminID,
				ActorIP:   c.IP(),
				Action:    audit.ActionUpdate,
				Outcome:   audit.OutcomeFailure,
				Reason:    "wrong_current_password",
				RequestID: reqID,
			})
			return c.Status(401).JSON(fiber.Map{"error": "current_password is incorrect"})
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}

		if _, err = db.Exec(ctx, `UPDATE admin_user SET password_hash = $1 WHERE id = $2`,
			string(newHash), actor.AdminID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.password_changed",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   actor.AdminID,
			Action:       audit.ActionUpdate,
			RequestID:    reqID,
		})

		return c.JSON(fiber.Map{"message": "password changed successfully"})
	}
}
