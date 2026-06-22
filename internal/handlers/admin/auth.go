package admin

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/audit"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates an admin user and returns a JWT
func Login(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		reqID, _ := c.Locals("request_id").(string)
		ip := c.IP()

		var req loginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Email == "" || req.Password == "" {
			return c.Status(400).JSON(fiber.Map{"error": "email and password are required"})
		}

		var (
			id           string
			name         string
			role         string
			passwordHash string
			isActive     bool
		)

		err := db.QueryRow(context.Background(),
			`SELECT id, name, role, password_hash, is_active FROM admin_user WHERE email = $1`,
			req.Email,
		).Scan(&id, &name, &role, &passwordHash, &isActive)

		if err != nil {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType: audit.EventAuthLoginFailed,
				ActorType: audit.ActorAnonymous,
				ActorIP:   ip,
				Action:    audit.ActionLogin,
				Outcome:   audit.OutcomeFailure,
				Reason:    "user_not_found",
				RequestID: reqID,
				Metadata:  map[string]any{"email": req.Email, "portal": "admin"},
			})
			return c.Status(401).JSON(fiber.Map{"error": "invalid_credentials"})
		}
		if !isActive {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType:  audit.EventAuthLoginFailed,
				ActorType:  audit.ActorAdmin,
				ActorID:    id,
				ActorIP:    ip,
				Action:     audit.ActionLogin,
				Outcome:    audit.OutcomeDenied,
				Reason:     "account_disabled",
				RequestID:  reqID,
			})
			return c.Status(403).JSON(fiber.Map{"error": "account_disabled"})
		}
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType: audit.EventAuthLoginFailed,
				ActorType: audit.ActorAdmin,
				ActorID:   id,
				ActorIP:   ip,
				Action:    audit.ActionLogin,
				Outcome:   audit.OutcomeFailure,
				Reason:    "invalid_password",
				RequestID: reqID,
			})
			return c.Status(401).JSON(fiber.Map{"error": "invalid_credentials"})
		}

		// Update last_login_at
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = db.Exec(ctx, `UPDATE admin_user SET last_login_at = NOW() WHERE id = $1`, id)
		}()

		token, err := issueAdminJWT(id, req.Email, name, role)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "token_generation_failed"})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    audit.EventAuthLogin,
			ActorType:    audit.ActorAdmin,
			ActorID:      id,
			ActorIP:      ip,
			ActorUA:      c.Get("User-Agent"),
			ResourceType: "admin_user",
			ResourceID:   id,
			Action:       audit.ActionLogin,
			RequestID:    reqID,
			Metadata:     map[string]any{"role": role},
		})

		return c.JSON(fiber.Map{
			"token":      token,
			"expires_in": 86400,
			"user": fiber.Map{
				"id":    id,
				"email": req.Email,
				"name":  name,
				"role":  role,
			},
		})
	}
}

func issueAdminJWT(id, email, name, role string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "changeme-jwt-secret"
	}

	claims := jwt.MapClaims{
		"sub":   id,
		"email": email,
		"name":  name,
		"role":  role,
		"aud":   "admin",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Me returns the current admin's profile from JWT claims
func Me() fiber.Handler {
	return func(c *fiber.Ctx) error {
		admin := c.Locals("admin")
		return c.JSON(admin)
	}
}
