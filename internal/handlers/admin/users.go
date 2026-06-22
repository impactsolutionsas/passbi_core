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

type adminUserRow struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// ListAdminUsers GET /api/admin/users — superadmin only
func ListAdminUsers(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT id, email, name, role, is_active, created_at, updated_at, last_login_at
			FROM admin_user
			ORDER BY created_at DESC
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		users := []adminUserRow{}
		for rows.Next() {
			var u adminUserRow
			_ = rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsActive,
				&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
			users = append(users, u)
		}
		return c.JSON(users)
	}
}

// GetAdminUser GET /api/admin/users/:id — superadmin only
func GetAdminUser(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Params("id")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var u adminUserRow
		err := db.QueryRow(ctx, `
			SELECT id, email, name, role, is_active, created_at, updated_at, last_login_at
			FROM admin_user WHERE id = $1
		`, userID).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsActive,
			&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "user_not_found"})
		}
		return c.JSON(u)
	}
}

type createAdminUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// CreateAdminUser POST /api/admin/users — superadmin only
func CreateAdminUser(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		reqID, _ := c.Locals("request_id").(string)

		var req createAdminUserRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Email == "" || req.Name == "" || req.Password == "" {
			return c.Status(400).JSON(fiber.Map{"error": "email, name, and password are required"})
		}
		if req.Role == "" {
			req.Role = "admin"
		}
		if !validAdminRole(req.Role) {
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid_role",
				"valid": []string{"superadmin", "admin", "ops"},
			})
		}
		if len(req.Password) < 8 {
			return c.Status(400).JSON(fiber.Map{"error": "password must be at least 8 characters"})
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var newID string
		err = db.QueryRow(ctx, `
			INSERT INTO admin_user (email, name, role, password_hash)
			VALUES ($1, $2, $3, $4) RETURNING id
		`, req.Email, req.Name, req.Role, string(hash)).Scan(&newID)
		if err != nil {
			if isDuplicateKey(err) {
				return c.Status(409).JSON(fiber.Map{"error": "email_already_exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.created",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   newID,
			Action:       audit.ActionCreate,
			AfterState:   map[string]any{"email": req.Email, "name": req.Name, "role": req.Role},
			RequestID:    reqID,
		})

		return c.Status(201).JSON(fiber.Map{
			"id":    newID,
			"email": req.Email,
			"name":  req.Name,
			"role":  req.Role,
		})
	}
}

type updateAdminUserRequest struct {
	Name     *string `json:"name"`
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
}

// UpdateAdminUser PATCH /api/admin/users/:id — superadmin only
func UpdateAdminUser(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		userID := c.Params("id")
		reqID, _ := c.Locals("request_id").(string)

		if userID == actor.AdminID {
			return c.Status(400).JSON(fiber.Map{
				"error": "cannot_modify_self",
				"hint":  "Use /api/admin/me/profile to update your own profile",
			})
		}

		var req updateAdminUserRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Name == nil && req.Role == nil && req.IsActive == nil {
			return c.Status(400).JSON(fiber.Map{"error": "no fields to update"})
		}
		if req.Role != nil && !validAdminRole(*req.Role) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_role", "valid": []string{"superadmin", "admin", "ops"}})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var before adminUserRow
		err := db.QueryRow(ctx, `
			SELECT id, email, name, role, is_active, created_at, updated_at, last_login_at
			FROM admin_user WHERE id = $1
		`, userID).Scan(&before.ID, &before.Email, &before.Name, &before.Role, &before.IsActive,
			&before.CreatedAt, &before.UpdatedAt, &before.LastLoginAt)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "user_not_found"})
		}

		setClauses := []string{}
		args := []any{}
		i := 1
		if req.Name != nil {
			setClauses = append(setClauses, "name = $"+strconv.Itoa(i))
			args = append(args, *req.Name)
			i++
		}
		if req.Role != nil {
			setClauses = append(setClauses, "role = $"+strconv.Itoa(i))
			args = append(args, *req.Role)
			i++
		}
		if req.IsActive != nil {
			setClauses = append(setClauses, "is_active = $"+strconv.Itoa(i))
			args = append(args, *req.IsActive)
			i++
		}

		args = append(args, userID)
		_, err = db.Exec(ctx,
			"UPDATE admin_user SET "+strings.Join(setClauses, ", ")+" WHERE id = $"+strconv.Itoa(i),
			args...,
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.updated",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   userID,
			Action:       audit.ActionUpdate,
			BeforeState:  map[string]any{"name": before.Name, "role": before.Role, "is_active": before.IsActive},
			AfterState:   req,
			RequestID:    reqID,
		})

		return c.SendStatus(204)
	}
}

// DeactivateAdminUser DELETE /api/admin/users/:id — superadmin only (soft delete)
func DeactivateAdminUser(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		userID := c.Params("id")
		reqID, _ := c.Locals("request_id").(string)

		if userID == actor.AdminID {
			return c.Status(400).JSON(fiber.Map{"error": "cannot_deactivate_self"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tag, err := db.Exec(ctx, `UPDATE admin_user SET is_active = false WHERE id = $1`, userID)
		if err != nil || tag.RowsAffected() == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "user_not_found"})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.deactivated",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   userID,
			Action:       audit.ActionDelete,
			RequestID:    reqID,
		})

		return c.SendStatus(204)
	}
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ResetAdminPassword POST /api/admin/users/:id/reset-password — superadmin only
func ResetAdminPassword(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor := c.Locals("admin").(*middleware.AdminContext)
		userID := c.Params("id")
		reqID, _ := c.Locals("request_id").(string)

		var req resetPasswordRequest
		if err := c.BodyParser(&req); err != nil || len(req.NewPassword) < 8 {
			return c.Status(400).JSON(fiber.Map{"error": "new_password must be at least 8 characters"})
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tag, err := db.Exec(ctx, `UPDATE admin_user SET password_hash = $1 WHERE id = $2`, string(hash), userID)
		if err != nil || tag.RowsAffected() == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "user_not_found"})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    "admin_user.password_reset",
			ActorType:    audit.ActorAdmin,
			ActorID:      actor.AdminID,
			ActorIP:      c.IP(),
			ResourceType: "admin_user",
			ResourceID:   userID,
			Action:       audit.ActionUpdate,
			Reason:       "superadmin_forced_reset",
			RequestID:    reqID,
		})

		return c.JSON(fiber.Map{"message": "password reset successfully"})
	}
}

func validAdminRole(role string) bool {
	switch role {
	case "superadmin", "admin", "ops":
		return true
	}
	return false
}

func isDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate")
}
